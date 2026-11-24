package database

import (
	"context"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Test data of the bulk change: own tenants TE000431–TE000436 (the fixture tenants carry count assertions).

type bulkFx struct {
	t  *testing.T
	db Database
}

func newBulkFx(t *testing.T, tenants ...string) *bulkFx {
	t.Helper()
	db, err := GetTestDB(context.Background(), testDB)
	require.NoError(t, err)
	f := &bulkFx{t: t, db: db}
	clean := func() {
		for _, q := range []string{`DELETE FROM base.participant WHERE tenant = ANY($1)`, `DELETE FROM base.tariff WHERE tenant = ANY($1)`,
			`DELETE FROM base.eeg WHERE tenant = ANY($1)`} {
			_, err := testDB.DbInstance.Exec(q, pq.Array(tenants))
			require.NoError(t, err)
		}
	}
	clean()
	t.Cleanup(clean)
	return f
}

func (f *bulkFx) exec(q string, args ...interface{}) {
	f.t.Helper()
	_, err := testDB.DbInstance.Exec(q, args...)
	require.NoError(f.t, err)
}

// eeg inserts a community; creditor nil = no creditor id.
func (f *bulkFx) eeg(tenant string, creditor *string) {
	f.t.Helper()
	f.exec(`INSERT INTO base.eeg (tenant, name, description, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
		street, "streetNumber", city, zip, email, iban, bic, creditor_id)
		VALUES ($1::text, 'EEG '||$1::text, 'Bulk', 'RC'||$1::text, 'LOCAL', 'AT009999', 'Test Operator', 'AT00999900000TC'||$1::text,
		'Weg', '1', 'Linz', '4020', 'bulk@example.org', 'AT611904300234573201', 'BKAUATWW', $2)`, tenant, creditor)
}

type bulkFxMember struct {
	id, tenant, number string
	debit, ref         *string
	noBank, noIban     bool
	tariff             *string
}

func (f *bulkFx) member(m bulkFxMember) {
	f.t.Helper()
	var number interface{} = m.number
	if m.number == "" {
		number = nil
	}
	f.exec(`INSERT INTO base.participant (id, tenant, "participantNumber", firstname, lastname, status, "createdBy",
		"lastModifiedBy", "participantSince", "tariffId") VALUES ($1, $2, $3, 'Vor', 'Nach', 'ACTIVE', 'test', 'test', '2025-01-01', $4)`,
		m.id, m.tenant, number, m.tariff)
	if m.noBank {
		return
	}
	iban := "AT611904300234573201"
	if m.noIban {
		iban = ""
	}
	f.exec(`INSERT INTO base.bankaccount (participant_id, iban, mandate_reference, mandate_date, sepa_direct_debit)
		VALUES ($1, $2, $3, '2025-01-01', $4)`, m.id, iban, m.ref, m.debit)
}

func (f *bulkFx) meter(tenant, participantId, meterId, direction string, flag int, tariff *string) {
	f.t.Helper()
	f.exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, direction, status, flag, tariff_id)
		VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $6)`, meterId, participantId, tenant, direction, flag, tariff)
}

func (f *bulkFx) tariff(tenant, id, typ, status string) {
	f.t.Helper()
	f.exec(`INSERT INTO base.tariff (id, tenant, type, name, version, status) VALUES ($1, $2, $3::text, 'Tarif '||$3::text, 1, $4)`,
		id, tenant, typ, status)
}

// value reads one column of the member's bank account or member row (q has $1 = participant id).
func (f *bulkFx) value(q, id string) *string {
	f.t.Helper()
	var v *string
	require.NoError(f.t, testDB.DbInstance.Get(&v, q, id))
	return v
}

func (f *bulkFx) debitOf(id string) *string {
	return f.value(`SELECT sepa_direct_debit FROM base.bankaccount WHERE participant_id = $1`, id)
}

func (f *bulkFx) refOf(id string) *string {
	return f.value(`SELECT mandate_reference FROM base.bankaccount WHERE participant_id = $1`, id)
}

func (f *bulkFx) modifiedBy(id string) string {
	return *f.value(`SELECT "lastModifiedBy" FROM base.participant WHERE id = $1`, id)
}

func (f *bulkFx) eegOf(tenant string) *model.BulkEeg {
	f.t.Helper()
	eeg, err := f.db.BulkEeg(context.Background(), tenant)
	require.NoError(f.t, err)
	return eeg
}

// preview plans the rule; apply sends every changed hit of a fresh preview back (what admin does).
func (f *bulkFx) preview(tenant string, rule model.BulkRule) *model.BulkPlan {
	f.t.Helper()
	require.NoError(f.t, rule.Validate())
	p, err := f.db.BulkPreview(context.Background(), f.eegOf(tenant), &rule)
	require.NoError(f.t, err)
	return p
}

func itemsOf(p *model.BulkPlan) []*model.BulkApplyItem {
	var items []*model.BulkApplyItem
	for _, r := range p.Rows {
		if r.Class == model.BulkClassChanged {
			items = append(items, &model.BulkApplyItem{ParticipantId: r.ParticipantId, MeteringPointId: r.MeteringPointId,
				ExpectedOld: r.OldValue, ExpectedNew: r.NewValue})
		}
	}
	return items
}

func (f *bulkFx) apply(tenant string, rule model.BulkRule, items []*model.BulkApplyItem) (*model.BulkApplyResult, error) {
	f.t.Helper()
	require.NoError(f.t, rule.Validate())
	req := &model.BulkApplyRequest{Rule: &rule, Items: items}
	require.NoError(f.t, req.ValidateItems())
	return f.db.BulkApply(context.Background(), f.eegOf(tenant), "support-user", req)
}

func sp(s string) *string { return &s }
