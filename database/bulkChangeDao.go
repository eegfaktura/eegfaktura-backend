package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"at.ourproject/vfeeg-backend/model"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"gopkg.in/guregu/null.v4"
)

// Bulk change of the support (konzept-massenaenderungen.md, Tech Design E): preview reads, apply plans again on
// the locked state and writes in one transaction. Nothing but the master data is stored.

type BulkChangeRepository interface {
	BulkCommunities(ctx context.Context) ([]model.BulkCommunity, error)
	// BulkEeg finds the community of the tenant (case-insensitive); ErrBulkUnknownTenant if there is none.
	BulkEeg(ctx context.Context, tenant string) (*model.BulkEeg, error)
	BulkPreview(ctx context.Context, eeg *model.BulkEeg, rule *model.BulkRule) (*model.BulkPlan, error)
	BulkApply(ctx context.Context, eeg *model.BulkEeg, user string, req *model.BulkApplyRequest) (*model.BulkApplyResult, error)
}

// ErrBulkOutcomeUnknown marks a failure of the COMMIT of apply: the client cannot be told "nothing changed".
var ErrBulkOutcomeUnknown = errors.New("bulk change commit failed")

const (
	// bulkStatementTimeout bounds every statement of preview and apply (SET LOCAL via set_config),
	// bulkApplyDeadline the whole transaction; both stay below the ingress timeout of 175 s.
	bulkStatementTimeout = "30s"
	bulkApplyDeadline    = 120 * time.Second
)

func (db *sqlDatabase) BulkCommunities(ctx context.Context) ([]model.BulkCommunity, error) {
	list := []model.BulkCommunity{}
	if err := db.db.SelectContext(ctx, &list, `SELECT tenant, name, "rcNumber" FROM base.eeg ORDER BY lower(name), tenant`); err != nil {
		return nil, model.ErrBulkChange(err)
	}
	return list, nil
}

func (db *sqlDatabase) BulkEeg(ctx context.Context, tenant string) (*model.BulkEeg, error) {
	return loadBulkEeg(ctx, db.db, tenant)
}

// BulkPreview plans without locks and writes nothing; one read-only snapshot, so the counts fit together. Same
// statement timeout and deadline as apply.
func (db *sqlDatabase) BulkPreview(ctx context.Context, eeg *model.BulkEeg, rule *model.BulkRule) (*model.BulkPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, bulkApplyDeadline)
	defer cancel()
	tx, err := db.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, model.ErrOpenTx(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := setBulkStatementTimeout(ctx, tx); err != nil {
		return nil, err
	}
	s, err := loadBulkSnapshot(ctx, tx, eeg, rule)
	if err != nil {
		return nil, err
	}
	return model.PlanBulkChange(rule, s, civil.Today().String(), nil), nil
}

// BulkApply (E2): READ COMMITTED; statement timeout and deadline → advisory locks (409) → requested ids of the
// community (400) → the requested rows FOR UPDATE in fixed order → plan on the locked state, only the requested hits whose
// expected values still match → one UPDATE … FROM unnest per table, the expected old value in the WHERE, the
// count of rows compared. Any failure rolls everything back.
func (db *sqlDatabase) BulkApply(ctx context.Context, eeg *model.BulkEeg, user string, req *model.BulkApplyRequest) (*model.BulkApplyResult, error) {
	ctx, cancel := context.WithTimeout(ctx, bulkApplyDeadline)
	defer cancel()
	rule := req.Rule
	var result *model.BulkApplyResult
	err := inTx(ctx, db.db, func(tx *sqlx.Tx) error {
		if err := setBulkStatementTimeout(ctx, tx); err != nil {
			return err
		}
		withCreditor := rule.Field == model.BulkDebitType || rule.Field == model.BulkMandateReference
		if err := tryBulkLocks(ctx, tx, eeg.Tenant, eeg.CreditorNorm, withCreditor); err != nil {
			return err
		}
		if err := assertBulkItems(ctx, tx, eeg.Tenant, req); err != nil {
			return err
		}
		if err := lockBulkRows(ctx, tx, eeg.Tenant, req); err != nil {
			return err
		}
		s, err := loadBulkSnapshot(ctx, tx, eeg, rule)
		if err != nil {
			return err
		}
		items := make(map[string]*model.BulkApplyItem, len(req.Items))
		for _, it := range req.Items {
			items[model.BulkItemKey(it.ParticipantId, it.MeteringPointId)] = it
		}
		plan := model.PlanBulkChange(rule, s, civil.Today().String(), func(r *model.BulkRow) bool {
			it := items[r.Key()]
			return it != nil && rule.SameValue(it.ExpectedOld, r.OldValue) && rule.SameValue(it.ExpectedNew, r.NewValue)
		})
		result = bulkResult(plan, req)
		return writeBulkRows(ctx, tx, eeg.Tenant, user, rule.Field, result.Rows)
	})
	var ve *model.VfeegError
	if err != nil && !errors.As(err, &ve) {
		// only the COMMIT returns a plain error (inTx): whether it took effect is not certain
		return nil, model.ErrBulkChange(fmt.Errorf("%w: %v", ErrBulkOutcomeUnknown, err))
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// setBulkStatementTimeout is SET LOCAL statement_timeout with a bound value (set_config, local to the transaction).
func setBulkStatementTimeout(ctx context.Context, tx *sqlx.Tx) error {
	if _, err := tx.ExecContext(ctx, `SELECT set_config('statement_timeout', $1, true)`, bulkStatementTimeout); err != nil {
		return model.ErrBulkChange(err)
	}
	return nil
}

// bulkResult splits the requested hits into written and skipped (with reason) and keeps the hints of the plan.
func bulkResult(plan *model.BulkPlan, req *model.BulkApplyRequest) *model.BulkApplyResult {
	rows := make(map[string]*model.BulkRow, len(plan.Rows))
	for _, r := range plan.Rows {
		rows[r.Key()] = r
	}
	res := &model.BulkApplyResult{Skipped: []*model.BulkSkip{}, RunHints: plan.RunHints, Rows: []*model.BulkRow{}}
	for _, it := range req.Items {
		r := rows[model.BulkItemKey(it.ParticipantId, it.MeteringPointId)]
		switch {
		case r != nil && r.Class == model.BulkClassChanged:
			res.Rows = append(res.Rows, r)
		case r != nil && r.Class == model.BulkClassError:
			res.Skipped = append(res.Skipped, &model.BulkSkip{ParticipantId: it.ParticipantId, MeteringPointId: it.MeteringPointId, Reason: r.Error})
		default:
			res.Skipped = append(res.Skipped, &model.BulkSkip{ParticipantId: it.ParticipantId, MeteringPointId: it.MeteringPointId, Reason: model.BulkSkipChanged})
		}
	}
	res.Changed = len(res.Rows)
	return res
}

// lockBulkRows locks only the requested hits, in a fixed order: the members by id, then their bank account rows by
// id; for the metering point tariff the requested current metering points. Members outside the run stay editable
// in web while apply runs. Other members of the community are read unlocked for the uniqueness check: a single
// edit there at the same moment is not prevented (as for neighbouring communities, W-261123-mm1).
func lockBulkRows(ctx context.Context, tx *sqlx.Tx, tenant string, req *model.BulkApplyRequest) error {
	pids := make([]string, len(req.Items))
	mids := make([]string, len(req.Items))
	for i, it := range req.Items {
		pids[i], mids[i] = it.ParticipantId, it.MeteringPointId
	}
	type stmt struct {
		q    string
		args []interface{}
	}
	stmts := []stmt{{`SELECT id FROM base.participant WHERE tenant = $1 AND id = ANY($2::uuid[]) ORDER BY id FOR UPDATE`,
		[]interface{}{tenant, pq.Array(pids)}}}
	switch req.Rule.Field {
	case model.BulkMeterTariff:
		stmts = []stmt{{`SELECT 1 FROM base.meteringpoint WHERE tenant = $1 AND flag = 1
			AND (participant_id, metering_point_id) IN (SELECT * FROM unnest($2::uuid[], $3::text[]))
			ORDER BY participant_id, metering_point_id FOR UPDATE`, []interface{}{tenant, pq.Array(pids), pq.Array(mids)}}}
	case model.BulkDebitType, model.BulkMandateReference, model.BulkMandateDate:
		stmts = append(stmts, stmt{`SELECT b.id FROM base.bankaccount b JOIN base.participant p ON p.id = b.participant_id
			WHERE p.tenant = $1 AND p.id = ANY($2::uuid[]) ORDER BY b.id FOR UPDATE OF b`, []interface{}{tenant, pq.Array(pids)}})
	}
	for _, st := range stmts {
		if _, err := tx.ExecContext(ctx, st.q, st.args...); err != nil {
			return model.ErrBulkChange(err)
		}
	}
	return nil
}

// Fixed statements per field: no column name comes from the request.
var bulkBankStmt = map[model.BulkField]string{
	model.BulkDebitType: `UPDATE base.bankaccount b SET sepa_direct_debit = u.new
		FROM unnest($1::uuid[], $2::text[], $3::text[]) AS u(id, old, new)
		WHERE b.id = u.id AND b.sepa_direct_debit IS NOT DISTINCT FROM u.old
		AND b.participant_id IN (SELECT id FROM base.participant WHERE tenant = $4)`,
	model.BulkMandateReference: `UPDATE base.bankaccount b SET mandate_reference = u.new
		FROM unnest($1::uuid[], $2::text[], $3::text[]) AS u(id, old, new)
		WHERE b.id = u.id AND b.mandate_reference IS NOT DISTINCT FROM u.old
		AND b.participant_id IN (SELECT id FROM base.participant WHERE tenant = $4)`,
	model.BulkMandateDate: `UPDATE base.bankaccount b SET mandate_date = u.new::date
		FROM unnest($1::uuid[], $2::text[], $3::text[]) AS u(id, old, new)
		WHERE b.id = u.id AND b.mandate_date IS NOT DISTINCT FROM u.old::date
		AND b.participant_id IN (SELECT id FROM base.participant WHERE tenant = $4)`,
}

// writeBulkRows writes the changing hits: members before bank accounts (as updateParticipant), "last modified"
// from the token's user; metering points with modifiedBy/modifiedAt.
func writeBulkRows(ctx context.Context, tx *sqlx.Tx, tenant, user string, field model.BulkField, rows []*model.BulkRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, len(rows))
	mids := make([]string, len(rows))
	bankIds := make([]string, len(rows))
	olds := make([]null.String, len(rows))
	news := make([]null.String, len(rows))
	for i, r := range rows {
		ids[i], mids[i], bankIds[i] = r.ParticipantId, r.MeteringPointId, r.BankId
		olds[i], news[i] = null.StringFromPtr(r.OldValue), null.StringFromPtr(r.NewValue)
	}
	exec := func(q string, args ...interface{}) error {
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return model.ErrBulkChange(err)
		}
		if n, err := res.RowsAffected(); err != nil || n != int64(len(rows)) {
			return model.ErrBulkChange(fmt.Errorf("bulk change wrote %d of %d rows (%v)", n, len(rows), err))
		}
		return nil
	}
	switch field {
	case model.BulkMeterTariff:
		return exec(`UPDATE base.meteringpoint m SET tariff_id = u.new::uuid, "modifiedBy" = $5, "modifiedAt" = $6
			FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[]) AS u(pid, mid, old, new)
			WHERE m.tenant = $7 AND m.participant_id = u.pid AND m.metering_point_id = u.mid AND m.flag = 1
			AND m.tariff_id IS NOT DISTINCT FROM u.old::uuid`,
			pq.Array(ids), pq.Array(mids), pq.Array(olds), pq.Array(news), user, civil.Now(), tenant)
	case model.BulkParticipantTariff:
		return exec(`UPDATE base.participant p SET "tariffId" = u.new::uuid, "lastModifiedBy" = $4, "lastModifiedDate" = $5
			FROM unnest($1::uuid[], $2::text[], $3::text[]) AS u(id, old, new)
			WHERE p.id = u.id AND p.tenant = $6 AND p."tariffId" IS NOT DISTINCT FROM u.old::uuid`,
			pq.Array(ids), pq.Array(olds), pq.Array(news), user, civil.Today(), tenant)
	}
	if err := exec(`UPDATE base.participant SET "lastModifiedBy" = $2, "lastModifiedDate" = $3
		WHERE tenant = $4 AND id = ANY($1::uuid[])`, pq.Array(ids), user, civil.Today(), tenant); err != nil {
		return err
	}
	return exec(bulkBankStmt[field], pq.Array(bankIds), pq.Array(olds), pq.Array(news), tenant)
}
