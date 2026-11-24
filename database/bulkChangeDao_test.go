package database

import (
	"context"
	"errors"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bxA = "TE000431" // creditor "AT12 ZZZ 0001"
	bxB = "TE000432" // same creditor id, other spelling
	bxC = "TE000433" // no creditor id
	bxD = "TE000434" // creditor id of blanks only
)

func bxId(n string) string { return "b4310000-0000-4000-8000-0000000000" + n }

func vfCode(t *testing.T, err error) int {
	t.Helper()
	var ve *model.VfeegError
	require.True(t, errors.As(err, &ve), "want a VfeegError, got %v", err)
	return ve.Code
}

var bxDebitWhereNone = model.BulkRule{Field: model.BulkDebitType, Action: model.BulkSet, Value: "CORE",
	Condition: model.BulkEquals, ConditionValue: "NONE"}

// Preview writes nothing; apply changes exactly the previewed hits, sets "last modified" from the token's user
// and the next billing (billing_masterdata) sees the new direct debit type.
func TestBulkChange_debitPreviewAndApply(t *testing.T) {
	f := newBulkFx(t, bxA)
	f.eeg(bxA, sp("AT12 ZZZ 0001"))
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", debit: sp("NONE"), ref: sp("M-1")})
	f.member(bulkFxMember{id: bxId("02"), tenant: bxA, number: "2", debit: sp("B2B"), ref: sp("M-2")})
	f.member(bulkFxMember{id: bxId("03"), tenant: bxA, number: "3", debit: sp("CORE"), ref: sp("M-3")})
	f.member(bulkFxMember{id: bxId("04"), tenant: bxA, number: "4", debit: sp(""), ref: sp("M-4")})

	p := f.preview(bxA, bxDebitWhereNone)
	assert.Equal(t, model.BulkCounts{Changed: 1, UnchangedCondition: 2, UnchangedTarget: 1}, p.Counts)
	assert.Equal(t, "NONE", *f.debitOf(bxId("01")), "the preview writes nothing")
	assert.Equal(t, "test", f.modifiedBy(bxId("01")))

	res, err := f.apply(bxA, bxDebitWhereNone, itemsOf(p))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Changed)
	assert.Empty(t, res.Skipped)
	assert.Equal(t, []string{"L3"}, res.RunHints)
	assert.Equal(t, "CORE", *f.debitOf(bxId("01")))
	assert.Equal(t, "support-user", f.modifiedBy(bxId("01")))
	assert.Equal(t, "test", f.modifiedBy(bxId("02")), "members outside the run stay untouched")

	var billed string
	require.NoError(t, testDB.DbInstance.Get(&billed,
		`SELECT participant_sepa_direct_debit FROM base.billing_masterdata WHERE participant_id = $1`, bxId("01")))
	assert.Equal(t, "CORE", billed)
}

// Uniqueness over the communities with the same normalised creditor id; the error text is the code only.
func TestBulkChange_uniqueAcrossCreditorId(t *testing.T) {
	f := newBulkFx(t, bxA, bxB, bxC, bxD)
	f.eeg(bxA, sp("AT12 ZZZ 0001"))
	f.eeg(bxB, sp(" at12zzz0001 "))
	f.eeg(bxC, nil)
	f.eeg(bxD, sp("   "))
	// B: a direct debit member uses X-1, a member without direct debit X-2
	f.member(bulkFxMember{id: bxId("11"), tenant: bxB, number: "B1", debit: sp("CORE"), ref: sp("x-1")})
	f.member(bulkFxMember{id: bxId("12"), tenant: bxB, number: "B2", debit: sp("NONE"), ref: sp("X-2")})
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", debit: sp("CORE"), ref: sp("old-1")})
	f.member(bulkFxMember{id: bxId("02"), tenant: bxA, number: "2", debit: sp("CORE"), ref: sp("old-2")})
	copyX := model.BulkRule{Field: model.BulkMandateReference, Action: model.BulkCopy, Source: model.BulkSourceParticipantNumber,
		Prefix: "X-", Condition: model.BulkAll}

	p := f.preview(bxA, copyX)
	rows := map[string]*model.BulkRow{}
	for _, r := range p.Rows {
		rows[r.ParticipantId] = r
	}
	assert.Equal(t, "F2", rows[bxId("01")].Error, "X-1 is used in B (case-insensitive)")
	assert.Equal(t, model.BulkClassChanged, rows[bxId("02")].Class, "X-2 in B has no direct debit")
	assert.Len(t, p.Rows, 2, "no member of B in the answer")

	res, err := f.apply(bxA, copyX, itemsOf(p))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Changed)
	assert.Equal(t, "X-2", *f.refOf(bxId("02")))
	assert.Equal(t, "old-1", *f.refOf(bxId("01")))
	assert.Equal(t, "x-1", *f.refOf(bxId("11")), "other communities are only read")

	t.Run("no creditor id: only the own community", func(t *testing.T) {
		f.member(bulkFxMember{id: bxId("21"), tenant: bxC, number: "1", debit: sp("CORE"), ref: sp("c")})
		f.member(bulkFxMember{id: bxId("31"), tenant: bxD, number: "D1", debit: sp("CORE"), ref: sp("X-1")})
		p := f.preview(bxC, copyX)
		require.Len(t, p.Rows, 1)
		assert.Equal(t, model.BulkClassChanged, p.Rows[0].Class, "X-1 of A's neighbours and of D do not count")
		p = f.preview(bxD, copyX)
		assert.Equal(t, model.BulkClassChanged, p.Rows[0].Class, "blank creditor ids are no shared creditor id")
	})
}

// ” is not NULL: clearing ” stores NULL and billing then takes the member number.
func TestBulkChange_clearReferenceStoresNull(t *testing.T) {
	f := newBulkFx(t, bxA)
	f.eeg(bxA, nil)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "135", debit: sp("CORE"), ref: sp("")})
	f.member(bulkFxMember{id: bxId("02"), tenant: bxA, number: "136", debit: sp("CORE")})
	clear := model.BulkRule{Field: model.BulkMandateReference, Action: model.BulkClear, Condition: model.BulkAll}
	p := f.preview(bxA, clear)
	assert.Equal(t, model.BulkCounts{Changed: 1, WithHint: 1, UnchangedTarget: 1}, p.Counts)
	assert.Equal(t, []string{"H6"}, p.Rows[0].Hints, "the effective reference changes from '' to the member number")
	_, err := f.apply(bxA, clear, itemsOf(p))
	require.NoError(t, err)
	assert.Nil(t, f.refOf(bxId("01")))
	var billed string
	require.NoError(t, testDB.DbInstance.Get(&billed,
		`SELECT participant_sepa_mandate_reference FROM base.billing_masterdata WHERE participant_id = $1`, bxId("01")))
	assert.Equal(t, "135", billed)
}

// Unknown, foreign ids reject the whole request (400 "preview outdated"), nothing is written.
func TestBulkChange_applyRejectsForeignAndUnknownIds(t *testing.T) {
	f := newBulkFx(t, bxA, bxB)
	f.eeg(bxA, nil)
	f.eeg(bxB, nil)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", debit: sp("NONE")})
	f.member(bulkFxMember{id: bxId("11"), tenant: bxB, number: "1", debit: sp("NONE")})
	for name, id := range map[string]string{"foreign": bxId("11"), "unknown": bxId("99")} {
		t.Run(name, func(t *testing.T) {
			items := []*model.BulkApplyItem{{ParticipantId: bxId("01"), ExpectedOld: sp("NONE"), ExpectedNew: sp("CORE")},
				{ParticipantId: id, ExpectedOld: sp("NONE"), ExpectedNew: sp("CORE")}}
			_, err := f.apply(bxA, bxDebitWhereNone, items)
			assert.Equal(t, 6102, vfCode(t, err))
			assert.Equal(t, "NONE", *f.debitOf(bxId("01")))
			assert.Equal(t, "NONE", *f.debitOf(bxId("11")))
		})
	}
	_, err := f.db.BulkEeg(context.Background(), "TE000499")
	assert.Equal(t, 6103, vfCode(t, err), "unknown community")
	eeg, err := f.db.BulkEeg(context.Background(), "te000431")
	require.NoError(t, err)
	assert.Equal(t, bxA, eeg.Tenant, "case-insensitive")
}

// A value changed between preview and apply is skipped and named; a member that matches only now stays.
func TestBulkChange_changedSincePreview(t *testing.T) {
	f := newBulkFx(t, bxA)
	f.eeg(bxA, nil)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", debit: sp("NONE")})
	f.member(bulkFxMember{id: bxId("02"), tenant: bxA, number: "2", debit: sp("NONE")})
	f.member(bulkFxMember{id: bxId("03"), tenant: bxA, number: "3", debit: sp("B2B")})
	p := f.preview(bxA, bxDebitWhereNone)
	f.exec(`UPDATE base.bankaccount SET sepa_direct_debit = 'B2B' WHERE participant_id = $1`, bxId("02"))
	f.exec(`UPDATE base.bankaccount SET sepa_direct_debit = 'NONE' WHERE participant_id = $1`, bxId("03"))

	res, err := f.apply(bxA, bxDebitWhereNone, itemsOf(p))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Changed)
	require.Len(t, res.Skipped, 1)
	assert.Equal(t, model.BulkSkip{ParticipantId: bxId("02"), Reason: model.BulkSkipChanged}, *res.Skipped[0])
	assert.Equal(t, "B2B", *f.debitOf(bxId("02")))
	assert.Equal(t, "NONE", *f.debitOf(bxId("03")), "matches only after the preview")

	t.Run("copy: the source changed (expected new value)", func(t *testing.T) {
		copyRef := model.BulkRule{Field: model.BulkMandateReference, Action: model.BulkCopy, Source: model.BulkSourceParticipantNumber, Condition: model.BulkAll}
		p := f.preview(bxA, copyRef)
		f.exec(`UPDATE base.participant SET "participantNumber" = '1a' WHERE id = $1`, bxId("01"))
		res, err := f.apply(bxA, copyRef, itemsOf(p))
		require.NoError(t, err)
		assert.Equal(t, 2, res.Changed)
		require.Len(t, res.Skipped, 1)
		assert.Equal(t, model.BulkSkipChanged, res.Skipped[0].Reason)
		assert.Nil(t, f.refOf(bxId("01")))
	})
}

// A member without a bank account row is an error (F6); no row is created.
func TestBulkChange_noBankAccountRow(t *testing.T) {
	f := newBulkFx(t, bxA)
	f.eeg(bxA, nil)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1", noBank: true})
	rule := model.BulkRule{Field: model.BulkDebitType, Action: model.BulkSet, Value: "CORE", Condition: model.BulkAll}
	p := f.preview(bxA, rule)
	require.Len(t, p.Rows, 1)
	assert.Equal(t, "F6", p.Rows[0].Error)
	res, err := f.apply(bxA, rule, []*model.BulkApplyItem{{ParticipantId: bxId("01"), ExpectedNew: sp("CORE")}})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Changed)
	assert.Equal(t, "F6", res.Skipped[0].Reason)
	var n int
	require.NoError(t, testDB.DbInstance.Get(&n, `SELECT count(*) FROM base.bankaccount WHERE participant_id = $1`, bxId("01")))
	assert.Zero(t, n)
}
