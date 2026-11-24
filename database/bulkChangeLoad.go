package database

import (
	"context"
	"database/sql"
	"errors"

	"at.ourproject/vfeeg-backend/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Reads of the bulk change (konzept-massenaenderungen.md B): the snapshot the planner works on. All values are
// bound parameters. Other communities are only read for the uniqueness of the mandate reference.

// creditorNormSQL is the normalised creditor id (no blanks, upper-cased), built in SQL once and used for the lock
// key and the set of communities alike.
const creditorNormSQL = `upper(regexp_replace(COALESCE(e.creditor_id, ''), '\s', '', 'g'))`

// The reads of the snapshot (also explained by the load check, bulkChangeLoadCheck_test.go).
const (
	bulkMembersSQL = `SELECT p.id::text AS id, p."participantNumber" AS participant_number,
		to_char(p."participantSince", 'YYYY-MM-DD') AS participant_since, p.firstname, p.lastname,
		p."businessRole" AS business_role, p."tariffId"::text AS tariff_id, b.id::text AS bank_id,
		COALESCE(btrim(b.iban), '') <> '' AS has_iban, b.mandate_reference,
		to_char(b.mandate_date, 'YYYY-MM-DD') AS mandate_date, b.sepa_direct_debit
		FROM base.participant p
		LEFT JOIN (SELECT DISTINCT ON (ba.participant_id) ba.* FROM base.bankaccount ba
		           JOIN base.participant pp ON pp.id = ba.participant_id AND pp.tenant = $1
		           ORDER BY ba.participant_id, ba.id) b ON b.participant_id = p.id
		WHERE p.tenant = $1
		ORDER BY p."participantNumber", p.id`
	bulkMetersSQL = `SELECT m.participant_id::text AS participant_id, m.metering_point_id,
		m.direction, m.tariff_id::text AS tariff_id
		FROM base.meteringpoint m JOIN base.participant p ON p.id = m.participant_id
		WHERE m.tenant = $1 AND m.flag = $2 AND m.direction = $3
		ORDER BY p."participantNumber", m.metering_point_id, m.participant_id`
	bulkTariffsSQL = `SELECT t.id::text AS id, t.name, t.type, a.id IS NOT NULL AS active,
		COALESCE(t."useVat", false) AS use_vat, t."vatInPercent"::float8 AS vat,
		t."useMeteringPointFee" AS use_meter_fee, t."meteringPointVat"::float8 AS meter_vat,
		t."joiningFee"::float8 AS joining_fee, t."joiningFeeUseVat" AS joining_fee_use_vat,
		t."joiningFeeVatInPercent"::float8 AS joining_fee_vat
		FROM (SELECT DISTINCT ON (id) * FROM base.tariff WHERE tenant = $1 ORDER BY id, version DESC) t
		LEFT JOIN base.activetariff a ON a.id = t.id AND a.version = t.version`
	// raw values: the planner normalises them like the own members (one rule, in Go)
	bulkForeignRefsSQL = `SELECT COALESCE(b.mandate_reference, p."participantNumber") AS ref, b.sepa_direct_debit
		FROM base.eeg e
		JOIN base.participant p ON p.tenant = e.tenant
		JOIN base.bankaccount b ON b.participant_id = p.id
		WHERE ` + creditorNormSQL + ` = $1 AND e.tenant <> $2`
)

// loadBulkEeg finds the community case-insensitively (an exact match first); ErrBulkUnknownTenant if none.
func loadBulkEeg(ctx context.Context, q sqlx.QueryerContext, tenant string) (*model.BulkEeg, error) {
	var eeg model.BulkEeg
	err := sqlx.GetContext(ctx, q, &eeg, `SELECT e.tenant, e.name, e."rcNumber", `+creditorNormSQL+` AS creditor_norm,
			COALESCE(btrim(e.iban), '') <> '' AS has_iban, COALESCE(btrim(e.bic), '') <> '' AS has_bic
		FROM base.eeg e WHERE upper(e.tenant) = upper($1) ORDER BY (e.tenant = $1) DESC LIMIT 1`, tenant)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrBulkUnknownTenant(errors.New("Gemeinschaft nicht gefunden"))
	}
	if err != nil {
		return nil, model.ErrBulkChange(err)
	}
	return &eeg, nil
}

// loadBulkSnapshot reads what the rule needs: members with their bank account row (the first by id; Prod has no
// member with two), the current meters of the direction, the tariffs of the community and, for rules on the
// direct debit type or the mandate reference, the references used under the same creditor id elsewhere.
func loadBulkSnapshot(ctx context.Context, q sqlx.QueryerContext, eeg *model.BulkEeg, rule *model.BulkRule) (*model.BulkSnapshot, error) {
	s := &model.BulkSnapshot{Eeg: eeg, Tariffs: map[string]*model.BulkTariff{}}
	err := sqlx.SelectContext(ctx, q, &s.Members, bulkMembersSQL, eeg.Tenant)
	if err != nil {
		return nil, model.ErrBulkChange(err)
	}
	if rule.Field == model.BulkMeterTariff {
		err = sqlx.SelectContext(ctx, q, &s.Meters, bulkMetersSQL, eeg.Tenant, model.F_ASSIGNED, rule.Direction)
		if err != nil {
			return nil, model.ErrBulkChange(err)
		}
	}
	var tariffs []*model.BulkTariff
	err = sqlx.SelectContext(ctx, q, &tariffs, bulkTariffsSQL, eeg.Tenant)
	if err != nil {
		return nil, model.ErrBulkChange(err)
	}
	for _, t := range tariffs {
		s.Tariffs[t.Id] = t
	}
	if (rule.Field == model.BulkDebitType || rule.Field == model.BulkMandateReference) && eeg.CreditorNorm != "" {
		if err = sqlx.SelectContext(ctx, q, &s.Foreign, bulkForeignRefsSQL, eeg.CreditorNorm, eeg.Tenant); err != nil {
			return nil, model.ErrBulkChange(err)
		}
	}
	return s, nil
}

// assertBulkItems loads all requested hits with one query on the community and compares the count: an unknown or
// foreign id rejects the whole request before anything is written (ErrBulkStalePreview, neutral text).
func assertBulkItems(ctx context.Context, tx *sqlx.Tx, tenant string, req *model.BulkApplyRequest) error {
	pids := make([]string, len(req.Items))
	mids := make([]string, len(req.Items))
	for i, it := range req.Items {
		pids[i], mids[i] = it.ParticipantId, it.MeteringPointId
	}
	var n int
	var err error
	if req.Rule.Field == model.BulkMeterTariff {
		err = tx.GetContext(ctx, &n, `SELECT count(*) FROM (SELECT DISTINCT participant_id, metering_point_id
			FROM base.meteringpoint WHERE tenant = $1
			AND (participant_id, metering_point_id) IN (SELECT * FROM unnest($2::uuid[], $3::text[]))) x`,
			tenant, pq.Array(pids), pq.Array(mids))
	} else {
		err = tx.GetContext(ctx, &n, `SELECT count(*) FROM base.participant WHERE tenant = $1 AND id = ANY($2::uuid[])`,
			tenant, pq.Array(pids))
	}
	if err != nil {
		return model.ErrBulkChange(err)
	}
	if n != len(req.Items) {
		return model.ErrBulkStalePreview(errors.New("Die Vorschau ist veraltet: bitte neu berechnen."))
	}
	return nil
}
