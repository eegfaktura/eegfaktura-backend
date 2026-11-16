package database

import (
	"context"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const migrationBeforeJoiningFeeText = 20261106100000
const joiningFeeTextMigration = 20261116090000

// Migration 20261116090000 (platform#260): base.tariff gets "joiningFeeText" (default 'Beitrittsgebühr', also for
// existing rows), appended at the end of both views; all existing view columns keep name, type and position
// (billing reads billing_masterdata with m.*). Down restores the views and keeps the column with its values.
func TestJoiningFeeTextMigrationUpDown(t *testing.T) {
	dbx, m := migrationDB(t, "joining_fee_text_migration_test")
	require.NoError(t, m.Migrate(migrationBeforeJoiningFeeText))
	billingBefore := viewColumns(t, dbx, "billing_masterdata")
	activeBefore := viewColumns(t, dbx, "activetariff")

	// a tariff from before the migration gets the text billing printed until now
	const existing, changed = "5f0c1a2e-0b7d-4c55-9a51-7d3f2b6a0051", "5f0c1a2e-0b7d-4c55-9a51-7d3f2b6a0052"
	_, err := dbx.Exec(`INSERT INTO base.tariff (id, tenant, type, name, version, "joiningFee")
		VALUES ($1, 'TE000051', 'EEG', 'Beitrag', 0, 49.90)`, existing)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(joiningFeeTextMigration))
	billing := viewColumns(t, dbx, "billing_masterdata")
	require.Len(t, billing, len(billingBefore)+1)
	assert.Equal(t, billingBefore, billing[:len(billingBefore)], "existing columns unchanged, same order")
	assert.Equal(t, viewColumn{"tariff_joining_fee_text", "text"}, billing[len(billing)-1], "new column at the end")
	active := viewColumns(t, dbx, "activetariff")
	require.Len(t, active, len(activeBefore)+1)
	assert.Equal(t, activeBefore, active[:len(activeBefore)])
	assert.Equal(t, viewColumn{"joiningFeeText", "text"}, active[len(active)-1])

	var nullable, text string
	require.NoError(t, dbx.Get(&nullable, `SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = 'base' AND table_name = 'tariff' AND column_name = 'joiningFeeText'`))
	assert.Equal(t, "NO", nullable)
	require.NoError(t, dbx.Get(&text, `SELECT "joiningFeeText" FROM base.activetariff WHERE id = $1`, existing))
	assert.Equal(t, model.DefaultJoiningFeeText, text, "existing tariff")

	// a changed text survives the down migration
	_, err = dbx.Exec(`INSERT INTO base.tariff (id, tenant, type, name, version, "joiningFee", "joiningFeeText")
		VALUES ($1, 'TE000051', 'EEG', 'Beitrag 2', 0, 30, 'Einmalige Einrichtungsgebühr')`, changed)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(migrationBeforeJoiningFeeText))
	assert.Equal(t, billingBefore, viewColumns(t, dbx, "billing_masterdata"), "down: view as before")
	assert.Equal(t, activeBefore, viewColumns(t, dbx, "activetariff"), "down: view as before")
	require.NoError(t, dbx.Get(&text, `SELECT "joiningFeeText" FROM base.tariff WHERE id = $1`, changed))
	assert.Equal(t, "Einmalige Einrichtungsgebühr", text, "down keeps the column and its values")

	// up again finds the existing column
	require.NoError(t, m.Migrate(joiningFeeTextMigration))
	assert.Equal(t, billing, viewColumns(t, dbx, "billing_masterdata"))
	require.NoError(t, dbx.Get(&text, `SELECT "joiningFeeText" FROM base.activetariff WHERE id = $1`, changed))
	assert.Equal(t, "Einmalige Einrichtungsgebühr", text)
}

// The tariff write and read paths carry the invoice line text; each version keeps its own; the view hands the text
// of the member's active EEG tariff to billing and falls back to the default for a member without an EEG tariff.
func TestJoiningFeeTextTariffAndBillingView(t *testing.T) {
	db, err := GetDB(context.Background())
	require.NoError(t, err)
	const tenant = "TE000052"
	// own tenant: remove its rows afterwards, so the test also passes on a rerun against the same database
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.participant WHERE tenant = $1`, tenant)
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.tariff WHERE tenant = $1`, tenant)
	})

	// as the controller does: validate (which fills an empty text) before saving
	tariff := model.Tariff{Type: model.EEG, Name: "Beitrag mit Einrichtungsgebühr", ParticipantFee: 2, JoiningFee: 30}
	require.NoError(t, tariff.ValidateJoiningFee())
	require.NoError(t, db.AddTariff(context.Background(), tenant, "test", &tariff))
	id := tariff.Id.String()

	activeText := func() string {
		tariffs, err := db.GetTariff(context.Background(), tenant)
		require.NoError(t, err)
		for _, tr := range tariffs {
			if tr.Id.String() == id {
				return tr.JoiningFeeText
			}
		}
		t.Fatal("tariff not found")
		return ""
	}
	assert.Equal(t, model.DefaultJoiningFeeText, activeText(), "no text sent")

	update := tariff
	update.JoiningFeeText = "  Einmalige Einrichtungsgebühr "
	require.NoError(t, update.ValidateJoiningFee())
	require.NoError(t, db.AddTariff(context.Background(), tenant, "test", &update))
	assert.Equal(t, "Einmalige Einrichtungsgebühr", activeText(), "trimmed")

	history, err := db.GetTariffHistory(context.Background(), tenant, id)
	require.NoError(t, err)
	require.Len(t, history, 2)
	assert.Equal(t, "Einmalige Einrichtungsgebühr", history[0].JoiningFeeText)
	assert.Equal(t, model.DefaultJoiningFeeText, history[1].JoiningFeeText)

	const withFee, withoutTariff = "5f0c1a2e-0b7d-4c55-9a51-7d3f2b6a0053", "5f0c1a2e-0b7d-4c55-9a51-7d3f2b6a0054"
	_, err = testDB.DbInstance.Exec(`INSERT INTO base.participant (id, tenant, firstname, lastname, "createdBy",
		"lastModifiedBy", "tariffId") VALUES ($1, $3, 'Mit', 'Gebühr', 'test', 'test', $4),
		($2, $3, 'Ohne', 'Tarif', 'test', 'test', NULL)`, withFee, withoutTariff, tenant, id)
	require.NoError(t, err)
	view := func(participant string) (text string) {
		require.NoError(t, testDB.DbInstance.Get(&text,
			`SELECT tariff_joining_fee_text FROM base.billing_masterdata WHERE participant_id = $1`, participant))
		return text
	}
	assert.Equal(t, "Einmalige Einrichtungsgebühr", view(withFee))
	assert.Equal(t, model.DefaultJoiningFeeText, view(withoutTariff))
}
