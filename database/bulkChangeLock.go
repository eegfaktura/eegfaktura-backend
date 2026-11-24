package database

import (
	"context"
	"errors"
	"strings"

	"at.ourproject/vfeeg-backend/model"
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
