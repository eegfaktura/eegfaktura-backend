package database

import (
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bxEeg      = "b4319000-0000-4000-8000-000000000001"
	bxArchived = "b4319000-0000-4000-8000-000000000002"
	bxVzp      = "b4319000-0000-4000-8000-000000000003"
	bxEzp      = "b4319000-0000-4000-8000-000000000004"
	bxForeign  = "b4319000-0000-4000-8000-000000000005"
	bxMeter    = "AT0030000000000000000000000431001"
)

func bxTariffs(f *bulkFx) {
	f.tariff(bxA, bxEeg, "EEG", "ACTIVE")
	f.tariff(bxA, bxArchived, "EEG", "ARCHIVED")
	f.tariff(bxA, bxVzp, "VZP", "ACTIVE")
	f.tariff(bxA, bxEzp, "EZP", "ACTIVE")
	f.tariff(bxB, bxForeign, "EEG", "ACTIVE")
}

// Member tariff: type EEG is set with "last modified"; archived, wrong type or foreign tariff → F4; no tariff → H9.
func TestBulkChange_participantTariff(t *testing.T) {
	f := newBulkFx(t, bxA, bxB)
	f.eeg(bxA, nil)
	f.eeg(bxB, nil)
	bxTariffs(f)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1"})
	f.member(bulkFxMember{id: bxId("02"), tenant: bxA, number: "2", tariff: sp(bxEeg)})
	set := func(id string) model.BulkRule {
		return model.BulkRule{Field: model.BulkParticipantTariff, Action: model.BulkSet, Value: id, Condition: model.BulkAll}
	}
	tariffOf := func(id string) *string {
		return f.value(`SELECT "tariffId"::text FROM base.participant WHERE id = $1`, id)
	}

	for name, id := range map[string]string{"archived": bxArchived, "VZP": bxVzp, "EZP": bxEzp, "foreign": bxForeign} {
		t.Run("F4 "+name, func(t *testing.T) {
			p := f.preview(bxA, set(id))
			assert.Equal(t, "F4", p.Rows[0].Error)
			assert.Equal(t, 2, p.Counts.Errors, "both members")
		})
	}

	p := f.preview(bxA, set(bxEeg))
	assert.Equal(t, model.BulkCounts{Changed: 1, UnchangedTarget: 1}, p.Counts)
	assert.Equal(t, "Tarif EEG", p.Rows[0].NewText)
	_, err := f.apply(bxA, set(bxEeg), itemsOf(p))
	require.NoError(t, err)
	assert.Equal(t, bxEeg, *tariffOf(bxId("01")))
	assert.Equal(t, "support-user", f.modifiedBy(bxId("01")))

	clear := model.BulkRule{Field: model.BulkParticipantTariff, Action: model.BulkClear, Condition: model.BulkEquals, ConditionValue: bxEeg}
	p = f.preview(bxA, clear)
	assert.Equal(t, 2, p.Counts.Changed)
	assert.Equal(t, []string{"H9"}, p.Rows[0].Hints)
	res, err := f.apply(bxA, clear, itemsOf(p))
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed)
	assert.Nil(t, tariffOf(bxId("02")), "no tariff stores NULL")
}

// Metering point tariff: only current meters of the direction in the chosen community; VZP only for consumption.
func TestBulkChange_meterTariff(t *testing.T) {
	f := newBulkFx(t, bxA, bxB)
	f.eeg(bxA, nil)
	f.eeg(bxB, nil)
	bxTariffs(f)
	f.member(bulkFxMember{id: bxId("01"), tenant: bxA, number: "1"})
	f.member(bulkFxMember{id: bxId("02"), tenant: bxA, number: "2"})
	f.member(bulkFxMember{id: bxId("11"), tenant: bxB, number: "1"})
	f.meter(bxA, bxId("01"), bxMeter, "CONSUMPTION", 1, nil)
	f.meter(bxA, bxId("02"), bxMeter, "CONSUMPTION", 0, nil) // moved away: history row
	f.meter(bxA, bxId("01"), "AT0030000000000000000000000431002", "GENERATION", 1, nil)
	f.meter(bxB, bxId("11"), bxMeter, "CONSUMPTION", 1, nil) // the same metering point in another community
	tariffOf := func(tenant, pid, mid string) *string {
		var v *string
		require.NoError(t, testDB.DbInstance.Get(&v, `SELECT tariff_id::text FROM base.meteringpoint
			WHERE tenant = $1 AND participant_id = $2 AND metering_point_id = $3`, tenant, pid, mid))
		return v
	}
	rule := func(id, dir string) model.BulkRule {
		return model.BulkRule{Field: model.BulkMeterTariff, Action: model.BulkSet, Value: id, Condition: model.BulkEmpty, Direction: dir}
	}

	assert.Equal(t, "F4", f.preview(bxA, rule(bxEzp, "CONSUMPTION")).Rows[0].Error)
	assert.Equal(t, "F4", f.preview(bxA, rule(bxVzp, "GENERATION")).Rows[0].Error)

	p := f.preview(bxA, rule(bxVzp, "CONSUMPTION"))
	require.Len(t, p.Rows, 1, "only the current consumption meter")
	assert.Equal(t, bxMeter, p.Rows[0].MeteringPointId)
	res, err := f.apply(bxA, rule(bxVzp, "CONSUMPTION"), itemsOf(p))
	require.NoError(t, err)
	assert.Equal(t, 1, res.Changed)
	assert.Equal(t, bxVzp, *tariffOf(bxA, bxId("01"), bxMeter))
	assert.Nil(t, tariffOf(bxA, bxId("02"), bxMeter), "history row unchanged")
	assert.Nil(t, tariffOf(bxB, bxId("11"), bxMeter), "other community unchanged")
	assert.Equal(t, "support-user", *f.value(`SELECT "modifiedBy" FROM base.meteringpoint WHERE participant_id = $1
		AND metering_point_id = '`+bxMeter+`'`, bxId("01")))

	t.Run("a meter of another community is a stale preview", func(t *testing.T) {
		_, err := f.apply(bxA, rule(bxVzp, "CONSUMPTION"), []*model.BulkApplyItem{{ParticipantId: bxId("11"), MeteringPointId: bxMeter}})
		assert.Equal(t, 6102, vfCode(t, err))
	})
}
