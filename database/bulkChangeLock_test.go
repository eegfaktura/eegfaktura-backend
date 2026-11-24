package database

import (
	"errors"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/model"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdLock takes an advisory lock in a transaction of its own (a second backend replica, or archiveTariff) until the
// test ends; wait: the blocking form of archiveTariff.
func holdLock(t *testing.T, class int, key string, wait bool) {
	t.Helper()
	tx, err := testDB.DbInstance.Beginx()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	if wait {
		_, err = tx.Exec(`SELECT pg_advisory_xact_lock($1, hashtext($2))`, class, key)
		require.NoError(t, err)
		return
	}
	var got bool
	require.NoError(t, tx.Get(&got, `SELECT pg_try_advisory_xact_lock($1, hashtext($2))`, class, key))
	require.True(t, got)
}

// A second run on the community, a run on another community with the same creditor id (rules on direct debit type
// or mandate reference) and a run while a tariff is archived get 409; nothing is written.
func TestBulkChange_locks(t *testing.T) {
	f := newBulkFx(t, bxA, bxB)
	f.eeg(bxA, sp("AT12 ZZZ 0001"))
	f.eeg(bxB, sp("at12zzz0001"))
	bxTariffs(f)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", debit: sp("NONE")})
	debitItems := []*model.BulkApplyItem{{ParticipantId: bxId("01"), ExpectedOld: sp("NONE"), ExpectedNew: sp("CORE")}}
	tariffRule := model.BulkRule{Field: model.BulkParticipantTariff, Action: model.BulkSet, Value: bxEeg, Condition: model.BulkAll}
	tariffItems := []*model.BulkApplyItem{{ParticipantId: bxId("01"), ExpectedNew: sp(bxEeg)}}
	creditor := f.eegOf(bxB).CreditorNorm
	require.Equal(t, "AT12ZZZ0001", creditor)
	require.Equal(t, creditor, f.eegOf(bxA).CreditorNorm, "one normalised value for both spellings")

	t.Run("second run on the community", func(t *testing.T) {
		holdLock(t, bulkLockTenant, bxA, false)
		_, err := f.apply(bxA, bxDebitWhereNone, debitItems)
		assert.Equal(t, 6104, vfCode(t, err))
		_, err = f.apply(bxA, tariffRule, tariffItems)
		assert.Equal(t, 6104, vfCode(t, err))
	})
	t.Run("run on another community with the same creditor id", func(t *testing.T) {
		holdLock(t, bulkLockCreditor, creditor, false)
		_, err := f.apply(bxA, bxDebitWhereNone, debitItems)
		assert.Equal(t, 6104, vfCode(t, err), "direct debit type")
		copyRef := model.BulkRule{Field: model.BulkMandateReference, Action: model.BulkCopy, Source: model.BulkSourceParticipantNumber, Condition: model.BulkAll}
		_, err = f.apply(bxA, copyRef, []*model.BulkApplyItem{{ParticipantId: bxId("01"), ExpectedNew: sp("1")}})
		assert.Equal(t, 6104, vfCode(t, err), "mandate reference")
		assert.Equal(t, "NONE", *f.debitOf(bxId("01")))
		assert.Nil(t, f.refOf(bxId("01")))
	})
	t.Run("tariff archive running", func(t *testing.T) {
		holdLock(t, bulkLockTenant, bxA, true)
		_, err := f.apply(bxA, tariffRule, tariffItems)
		assert.Equal(t, 6104, vfCode(t, err))
		assert.Nil(t, f.value(`SELECT "tariffId"::text FROM base.participant WHERE id = $1`, bxId("01")))
	})
	t.Run("a tariff rule does not take the creditor lock", func(t *testing.T) {
		holdLock(t, bulkLockCreditor, creditor, false)
		res, err := f.apply(bxA, tariffRule, tariffItems)
		require.NoError(t, err)
		assert.Equal(t, 1, res.Changed)
	})
	t.Run("free again after the other run ended", func(t *testing.T) {
		res, err := f.apply(bxA, bxDebitWhereNone, debitItems)
		require.NoError(t, err)
		assert.Equal(t, 1, res.Changed)
	})
}

// A failure while writing rolls everything back: the member rows written first are unchanged too.
func TestBulkChange_applyIsAtomic(t *testing.T) {
	f := newBulkFx(t, bxA)
	f.eeg(bxA, nil)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", debit: sp("NONE")})
	f.member(bulkFxMember{id: bxId("02"), tenant: bxA, number: "2", debit: sp("NONE")})
	f.exec(`CREATE OR REPLACE FUNCTION base.bulk_test_fail() RETURNS trigger AS $$
		BEGIN IF NEW.participant_id = '` + bxId("02") + `' THEN RAISE EXCEPTION 'bulk test failure'; END IF; RETURN NEW; END
		$$ LANGUAGE plpgsql`)
	f.exec(`CREATE TRIGGER bulk_test_fail BEFORE UPDATE ON base.bankaccount FOR EACH ROW EXECUTE FUNCTION base.bulk_test_fail()`)
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DROP TRIGGER IF EXISTS bulk_test_fail ON base.bankaccount`)
		_, _ = testDB.DbInstance.Exec(`DROP FUNCTION IF EXISTS base.bulk_test_fail()`)
	})

	p := f.preview(bxA, bxDebitWhereNone)
	require.Equal(t, 2, p.Counts.Changed)
	_, err := f.apply(bxA, bxDebitWhereNone, itemsOf(p))
	assert.Equal(t, 6105, vfCode(t, err))
	assert.Equal(t, "NONE", *f.debitOf(bxId("01")))
	assert.Equal(t, "test", f.modifiedBy(bxId("01")), "the member update is rolled back too")
}

// Apply locks only the requested hits: while it runs, a single edit of another member of the community goes
// through at once; an edit of a hit waits (here: runs into the lock timeout).
func TestBulkChange_applyLocksOnlyRequestedRows(t *testing.T) {
	f := newBulkFx(t, bxA)
	f.eeg(bxA, nil)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", debit: sp("NONE")})
	f.member(bulkFxMember{id: bxId("02"), tenant: bxA, number: "2", debit: sp("NONE")})
	// keeps apply inside its transaction while the test edits
	f.exec(`CREATE OR REPLACE FUNCTION base.bulk_test_slow() RETURNS trigger AS $$
		BEGIN IF NEW.participant_id = '` + bxId("01") + `' THEN PERFORM pg_sleep(2); END IF; RETURN NEW; END
		$$ LANGUAGE plpgsql`)
	f.exec(`CREATE TRIGGER bulk_test_slow BEFORE UPDATE ON base.bankaccount FOR EACH ROW EXECUTE FUNCTION base.bulk_test_slow()`)
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DROP TRIGGER IF EXISTS bulk_test_slow ON base.bankaccount`)
		_, _ = testDB.DbInstance.Exec(`DROP FUNCTION IF EXISTS base.bulk_test_slow()`)
	})

	done := make(chan error, 1)
	go func() {
		_, err := f.apply(bxA, bxDebitWhereNone, []*model.BulkApplyItem{{ParticipantId: bxId("01"), ExpectedOld: sp("NONE"), ExpectedNew: sp("CORE")}})
		done <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var n int
		require.NoError(t, testDB.DbInstance.Get(&n, `SELECT count(*) FROM pg_stat_activity WHERE wait_event = 'PgSleep'`))
		if n > 0 {
			break
		}
		require.True(t, time.Now().Before(deadline), "apply did not reach the write")
		time.Sleep(20 * time.Millisecond)
	}

	edit := func(id string) error {
		tx, err := testDB.DbInstance.Beginx()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		if _, err = tx.Exec(`SET LOCAL lock_timeout = '500ms'`); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE base.participant SET lastname = 'Einzeln' WHERE id = $1`, id); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE base.bankaccount SET owner = 'Einzeln' WHERE participant_id = $1`, id); err != nil {
			return err
		}
		return tx.Commit()
	}
	assert.NoError(t, edit(bxId("02")), "a member outside the hits is not blocked")
	var pqErr *pq.Error
	if err := edit(bxId("01")); assert.True(t, errors.As(err, &pqErr), "a hit is locked: %v", err) {
		assert.Equal(t, "55P03", string(pqErr.Code))
	}

	require.NoError(t, <-done)
	assert.Equal(t, "CORE", *f.debitOf(bxId("01")))
	assert.Equal(t, "NONE", *f.debitOf(bxId("02")))
}

// R1-2: a failing COMMIT is reported as ErrBulkOutcomeUnknown (the client must not read "nothing changed").
func TestBulkChange_commitFailureOutcomeUnknown(t *testing.T) {
	f := newBulkFx(t, bxA)
	f.eeg(bxA, nil)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", debit: sp("NONE")})
	// a deferred constraint trigger fails only at COMMIT
	f.exec(`CREATE OR REPLACE FUNCTION base.bulk_test_commit_fail() RETURNS trigger AS $$
		BEGIN IF NEW.participant_id = '` + bxId("01") + `' THEN RAISE EXCEPTION 'bulk test commit failure'; END IF; RETURN NEW; END
		$$ LANGUAGE plpgsql`)
	f.exec(`CREATE CONSTRAINT TRIGGER bulk_test_commit_fail AFTER UPDATE ON base.bankaccount DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION base.bulk_test_commit_fail()`)
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DROP TRIGGER IF EXISTS bulk_test_commit_fail ON base.bankaccount`)
		_, _ = testDB.DbInstance.Exec(`DROP FUNCTION IF EXISTS base.bulk_test_commit_fail()`)
	})

	_, err := f.apply(bxA, bxDebitWhereNone, itemsOf(f.preview(bxA, bxDebitWhereNone)))
	var ve *model.VfeegError
	require.True(t, errors.As(err, &ve), "want a VfeegError, got %v", err)
	assert.Equal(t, 6105, ve.Code)
	assert.ErrorIs(t, ve.Err, ErrBulkOutcomeUnknown)
	assert.Equal(t, "NONE", *f.debitOf(bxId("01")), "this commit failure rolled back")
}
