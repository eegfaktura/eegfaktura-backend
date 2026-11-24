package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForAdvisoryWaiter polls until a session waits for an advisory lock of the class (no fixed sleep).
func waitForAdvisoryWaiter(t *testing.T, class int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		require.NoError(t, testDB.DbInstance.Get(&n,
			`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted AND classid::bigint = $1`, class))
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no session waits for the advisory lock")
}

// archiveTariff waits for the tenant lock of a running bulk change (konzept-massenaenderungen.md E4) and then sees
// its committed state: a tariff the bulk change assigned is in use (4002) and stays unarchived.
func TestArchiveTariff_waitsForBulkChange(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)

	const tenant = "TE000419"
	const partId = "a4190000-0000-0000-0000-000000000001"
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.participant WHERE tenant = $1`, tenant)
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.tariff WHERE tenant = $1`, tenant)
	})
	tariff := model.Tariff{Name: "Massenlauf", Type: "EEG", BillingPeriod: "monthly"}
	require.NoError(t, db.AddTariff(ctx, tenant, "test", &tariff))
	_, err = testDB.DbInstance.Exec(`INSERT INTO base.participant (id, tenant, firstname, lastname, status, "createdBy", "lastModifiedBy")
		VALUES ($1, $2, 'Ta', 'Rif', 'ACTIVE', 'test', 'test') ON CONFLICT DO NOTHING`, partId, tenant)
	require.NoError(t, err)

	// the "bulk change": holds the tenant lock and assigns the tariff, not committed yet
	bulk, err := testDB.DbInstance.Beginx()
	require.NoError(t, err)
	defer func() { _ = bulk.Rollback() }()
	var got bool
	require.NoError(t, bulk.Get(&got, `SELECT pg_try_advisory_xact_lock($1, hashtext($2))`, bulkLockTenant, tenant))
	require.True(t, got)
	_, err = bulk.Exec(`UPDATE base.participant SET "tariffId" = $1 WHERE id = $2`, tariff.Id.String(), partId)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- db.ArchiveTariff(ctx, tenant, tariff.Id.String()) }()
	waitForAdvisoryWaiter(t, bulkLockTenant)
	select {
	case err := <-done:
		t.Fatalf("archiveTariff did not wait for the bulk change: %v", err)
	default:
	}

	require.NoError(t, bulk.Commit())
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("archiveTariff still waits after the bulk change committed")
	}
	var ve *model.VfeegError
	require.True(t, errors.As(err, &ve), "want a VfeegError, got %v", err)
	assert.Equal(t, 4002, ve.Code, "the tariff the bulk change assigned is in use")

	var status string
	require.NoError(t, testDB.DbInstance.Get(&status, `SELECT DISTINCT status FROM base.tariff WHERE tenant = $1 AND id = $2`, tenant, tariff.Id.String()))
	assert.NotEqual(t, "ARCHIVED", status)
}

// The wait of archiveTariff is bounded: when a bulk change holds the lock longer, 4003 (409) and nothing archived.
func TestArchiveTariff_lockTimeout(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)
	orig := archiveLockTimeout
	archiveLockTimeout = "300ms"
	t.Cleanup(func() { archiveLockTimeout = orig })

	const tenant = "TE000420"
	t.Cleanup(func() { _, _ = testDB.DbInstance.Exec(`DELETE FROM base.tariff WHERE tenant = $1`, tenant) })
	tariff := model.Tariff{Name: "frei", Type: "EEG", BillingPeriod: "monthly"}
	require.NoError(t, db.AddTariff(ctx, tenant, "test", &tariff))

	bulk, err := testDB.DbInstance.Beginx()
	require.NoError(t, err)
	defer func() { _ = bulk.Rollback() }()
	_, err = bulk.Exec(`SELECT pg_advisory_xact_lock($1, hashtext($2))`, bulkLockTenant, tenant)
	require.NoError(t, err)

	start := time.Now()
	err = db.ArchiveTariff(ctx, tenant, tariff.Id.String())
	var ve *model.VfeegError
	require.True(t, errors.As(err, &ve), "want a VfeegError, got %v", err)
	assert.Equal(t, 4003, ve.Code)
	assert.Equal(t, "Gerade läuft eine Massenänderung, bitte später erneut versuchen.", model.UserText(err))
	assert.Less(t, time.Since(start), 5*time.Second)

	var status string
	require.NoError(t, testDB.DbInstance.Get(&status, `SELECT DISTINCT status FROM base.tariff WHERE tenant = $1 AND id = $2`, tenant, tariff.Id.String()))
	assert.NotEqual(t, "ARCHIVED", status)
}
