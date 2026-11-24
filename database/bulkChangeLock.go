package database

import (
	"context"
	"errors"
	"strings"

	"at.ourproject/vfeeg-backend/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
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

// archiveLockTimeout bounds the wait of archiveTariff for the tenant lock: just above bulkApplyDeadline, so it
// only runs out when a bulk change hangs. A variable for the tests.
var archiveLockTimeout = "130s"

// waitBulkTenantLock waits for the tenant lock (archiveTariff: an EEG admin gets no 409 while a bulk change runs,
// it waits for it). SET LOCAL lock_timeout first: if the wait runs out, ErrTariffLocked (409); other failures
// ErrUpdateTariff.
func waitBulkTenantLock(ctx context.Context, tx *sqlx.Tx, tenant string) error {
	if _, err := tx.ExecContext(ctx, `SELECT set_config('lock_timeout', $1, true)`, archiveLockTimeout); err != nil {
		return model.ErrUpdateTariff(err)
	}
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2))`, bulkLockTenant, strings.ToUpper(tenant))
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "55P03" { // lock_not_available
		return model.ErrTariffLocked(errors.New("Gerade läuft eine Massenänderung, bitte später erneut versuchen."))
	}
	if err != nil {
		return model.ErrUpdateTariff(err)
	}
	return nil
}

// tryBulkLocks takes the locks of a bulk change without waiting, in the fixed order tenant, then creditor id
// (withCreditor: rules on the direct debit type or the mandate reference, community with a creditor id). The
// boolean answer is read: a lock held by another run or by archiveTariff gives ErrBulkLocked (409). Try instead of
// wait: two runs cannot deadlock.
func tryBulkLocks(ctx context.Context, tx *sqlx.Tx, tenant, creditorNorm string, withCreditor bool) error {
	type lock struct {
		class int
		key   string
	}
	keys := []lock{{bulkLockTenant, strings.ToUpper(tenant)}}
	if withCreditor && creditorNorm != "" {
		keys = append(keys, lock{bulkLockCreditor, creditorNorm})
	}
	for _, k := range keys {
		var got bool
		if err := tx.GetContext(ctx, &got, `SELECT pg_try_advisory_xact_lock($1, hashtext($2))`, k.class, k.key); err != nil {
			return model.ErrBulkChange(err)
		}
		if !got {
			return model.ErrBulkLocked(errors.New("Für diese Gemeinschaft oder Gläubiger-ID läuft gerade eine " +
				"Massenänderung, oder ein Tarif wird archiviert. Bitte gleich noch einmal versuchen."))
		}
	}
	return nil
}
