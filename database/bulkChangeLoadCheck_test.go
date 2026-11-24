package database

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/require"
)

// Load check of the bulk change (Tech Design B, approved 24.11.2026): plans and timings of the snapshot reads on
// Prod-like volume. Not part of the normal run (and not of CI); start it with
//
//	BULK_LOAD=1 TZ=Europe/Berlin go test -count=1 -v -run TestBulkChangeLoadCheck ./database/
//
// Volume: 60 communities, 52,500 members with one bank account and one metering point each; 17 communities share
// one creditor id in three spellings; the chosen community TL000000 has 1,000 members.
func TestBulkChangeLoadCheck(t *testing.T) {
	if os.Getenv("BULK_LOAD") != "1" {
		t.Skip("load check: set BULK_LOAD=1")
	}
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)
	exec := func(q string) {
		_, err := testDB.DbInstance.Exec(q)
		require.NoError(t, err)
	}
	// base.bankaccount and base.meteringpoint have no index on participant_id: the ON DELETE CASCADE of 52,500
	// members would scan them once per member. The cleanup deletes the child rows itself, without FK triggers
	// (session_replication_role, the test user is the container's superuser).
	clean := func() {
		tx, err := testDB.DbInstance.Beginx()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		for _, q := range []string{`SET LOCAL session_replication_role = replica`,
			`DELETE FROM base.bankaccount WHERE participant_id IN (SELECT id FROM base.participant WHERE tenant LIKE 'TL%')`,
			`DELETE FROM base.meteringpoint WHERE tenant LIKE 'TL%'`,
			`DELETE FROM base.participant WHERE tenant LIKE 'TL%'`,
			`DELETE FROM base.tariff WHERE tenant LIKE 'TL%'`,
			`DELETE FROM base.eeg WHERE tenant LIKE 'TL%'`} {
			_, err := tx.Exec(q)
			require.NoError(t, err)
		}
		require.NoError(t, tx.Commit())
	}
	clean()
	t.Cleanup(clean)

	exec(`INSERT INTO base.eeg (tenant, name, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
		street, "streetNumber", city, zip, email, iban, bic, creditor_id)
		SELECT 'TL' || lpad(g::text, 6, '0'), 'Last ' || g, 'RCL' || g, 'LOCAL', 'AT009999', 'Op', 'CL' || g, 'Weg', '1',
		       'Linz', '4020', 'l@example.org', 'AT611904300234573201', 'BKAUATWW',
		       CASE WHEN g >= 17 THEN 'AT' || g || 'ZZZ' WHEN g % 3 = 0 THEN 'AT12 ZZZ 0001' WHEN g % 3 = 1 THEN 'at12zzz0001'
		            ELSE ' AT12ZZZ 0001 ' END
		FROM generate_series(0, 59) g`)
	exec(`INSERT INTO base.participant (id, tenant, "participantNumber", firstname, lastname, "createdBy", "lastModifiedBy")
		SELECT uuid_generate_v4(), 'TL000000', g::text, 'Vor', 'Nach', 'load', 'load' FROM generate_series(1, 1000) g`)
	exec(`INSERT INTO base.participant (id, tenant, "participantNumber", firstname, lastname, "createdBy", "lastModifiedBy")
		SELECT uuid_generate_v4(), 'TL' || lpad((1 + g % 59)::text, 6, '0'), g::text, 'Vor', 'Nach', 'load', 'load'
		FROM generate_series(1, 51500) g`)
	exec(`INSERT INTO base.bankaccount (participant_id, iban, mandate_reference, mandate_date, sepa_direct_debit)
		SELECT id, 'AT611904300234573201', CASE WHEN random() < 0.5 THEN NULL ELSE 'M-' || "participantNumber" END,
		       '2025-01-01', CASE WHEN random() < 0.5 THEN 'CORE' ELSE 'NONE' END
		FROM base.participant WHERE tenant LIKE 'TL%'`)
	exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, direction, status)
		SELECT 'AT00300000000000000000' || lpad(row_number() OVER ()::text, 11, '0'), id, tenant,
		       CASE WHEN random() < 0.8 THEN 'CONSUMPTION' ELSE 'GENERATION' END, 'ACTIVE'
		FROM base.participant WHERE tenant LIKE 'TL%'`)
	exec(`INSERT INTO base.tariff (id, tenant, type, name, version, status)
		SELECT uuid_generate_v4(), e.tenant, (ARRAY['EEG', 'VZP', 'EZP'])[1 + g % 3], 'T' || g, 1, 'ACTIVE'
		FROM base.eeg e, generate_series(0, 5) g WHERE e.tenant LIKE 'TL%'`)
	exec(`ANALYZE`)

	eeg, err := db.BulkEeg(ctx, "TL000000")
	require.NoError(t, err)
	require.Equal(t, "AT12ZZZ0001", eeg.CreditorNorm)
	explain := func(name, q string, args ...interface{}) {
		var lines []string
		require.NoError(t, testDB.DbInstance.Select(&lines, `EXPLAIN (ANALYZE, BUFFERS) `+q, args...))
		t.Logf("=== %s\n%s", name, strings.Join(lines, "\n"))
	}
	explain("foreign references (uniqueness)", bulkForeignRefsSQL, eeg.CreditorNorm, eeg.Tenant)
	explain("members with bank account", bulkMembersSQL, eeg.Tenant)
	explain("meters of a direction", bulkMetersSQL, eeg.Tenant, model.F_ASSIGNED, "CONSUMPTION")
	explain("tariffs", bulkTariffsSQL, eeg.Tenant)

	timed := func(name string, fn func() error) {
		start := time.Now()
		require.NoError(t, fn())
		t.Logf("=== %s: %v", name, time.Since(start))
	}
	debit := model.BulkRule{Field: model.BulkDebitType, Action: model.BulkSet, Value: "CORE", Condition: model.BulkEquals, ConditionValue: "NONE"}
	meter := model.BulkRule{Field: model.BulkMeterTariff, Action: model.BulkClear, Condition: model.BulkAll, Direction: "CONSUMPTION"}
	var plan *model.BulkPlan
	timed("preview DEBIT_TYPE (1,000 members, 17 communities on the creditor id)", func() (err error) {
		plan, err = db.BulkPreview(ctx, eeg, &debit)
		return err
	})
	timed("preview METER_TARIFF", func() error { _, err := db.BulkPreview(ctx, eeg, &meter); return err })
	req := &model.BulkApplyRequest{Rule: &debit}
	for _, r := range plan.Rows {
		if r.Class == model.BulkClassChanged {
			req.Items = append(req.Items, &model.BulkApplyItem{ParticipantId: r.ParticipantId, ExpectedOld: r.OldValue, ExpectedNew: r.NewValue})
		}
	}
	timed("apply DEBIT_TYPE ("+strconv.Itoa(len(req.Items))+" hits)", func() error {
		res, err := db.BulkApply(ctx, eeg, "load", req)
		if err == nil {
			require.Equal(t, len(req.Items), res.Changed)
		}
		return err
	})
}
