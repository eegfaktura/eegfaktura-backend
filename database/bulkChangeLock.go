package database

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Advisory locks of the bulk change (konzept-massenaenderungen.md E3/E4), transaction-scoped, in the two-key form
// (classid, hashtext(key)): they never meet the one-key locks of the mail templates (mailTemplateDao.go) or of the
// energy requests (energyRequestDao.go). A hash collision of two keys only makes a run wait or get 409 needlessly.
const (
	// bulkLockTenant: one key per community (upper-cased tenant); taken by a bulk change and by archiveTariff.
	bulkLockTenant = 0x42430001
	// bulkLockCreditor: one key per normalised creditor id; taken by a bulk change on the direct debit type or
	// the mandate reference, as communities sharing the creditor id share the uniqueness of mandate references.
	bulkLockCreditor = 0x42430002
)

// waitBulkTenantLock waits for the tenant lock (archiveTariff: an EEG admin gets no 409, it waits for a running
// bulk change).
func waitBulkTenantLock(ctx context.Context, tx *sqlx.Tx, tenant string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2))`, bulkLockTenant, strings.ToUpper(tenant))
	return err
}
