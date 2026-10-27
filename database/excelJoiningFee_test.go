package database

import (
	"bytes"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gopkg.in/guregu/null.v4"
)

// exportRows reads the sheet "Mitglieder" of an exported workbook (raw values).
func exportRows(t *testing.T, buf *bytes.Buffer) [][]string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows("Mitglieder", excelize.Options{RawCellValue: true})
	require.NoError(t, err)
	return rows
}

func column(t *testing.T, header []string, name string) int {
	t.Helper()
	for i, h := range header {
		if h == name {
			return i
		}
	}
	t.Fatalf("column %q not in header %v", name, header)
	return -1
}

// Member export, konzept-beitrittsgebuehr.md: "Beitrittsgebühr (€)" and "Beitrittsgebühr verrechnet" directly
// after "Mitgliedstarif", in every row of the member; the list from web only fills "verrechnet" for the
// tenant's own members.
func TestExportMasterdataToExcel_joiningFee(t *testing.T) {
	const feeTariff, plainTariff = "7a1d0c2e-1111-4c55-9a51-7d3f2b6a0001", "7a1d0c2e-1111-4c55-9a51-7d3f2b6a0002"
	withFee := &model.EegParticipant{EegParticipantBase: model.EegParticipantBase{
		Id: uuid.Parse("7a1d0c2e-2222-4c55-9a51-7d3f2b6a0001"), FirstName: "Mit", LastName: "Gebühr",
		TariffId:      null.StringFrom(feeTariff),
		MeteringPoint: []*model.MeteringPoint{{MeteringPoint: "AT0030000000000000000000000000001", State: &model.MeterState{}}, {MeteringPoint: "AT0030000000000000000000000000002", State: &model.MeterState{}}},
	}}
	withoutFee := &model.EegParticipant{EegParticipantBase: model.EegParticipantBase{
		Id: uuid.Parse("7a1d0c2e-2222-4c55-9a51-7d3f2b6a0002"), FirstName: "Ohne", LastName: "Gebühr",
		TariffId:      null.StringFrom(plainTariff),
		MeteringPoint: []*model.MeteringPoint{{MeteringPoint: "AT0030000000000000000000000000003", State: &model.MeterState{}}},
	}}
	participants := []*model.EegParticipant{withFee, withoutFee}
	eeg := &model.Eeg{RcNumber: "RC100999"}
	tariffNames := map[string]string{feeTariff: "Beitrag mit Gebühr", plainTariff: "Beitrag"}
	joiningFees := map[string]float64{feeTariff: 49.9}

	states := []model.JoiningFeeState{
		// id in upper case: matched case-insensitively
		{ParticipantId: strings.ToUpper(withFee.Id.String()), EnergyBilled: true,
			JoiningFee: &model.JoiningFeeDocument{DocumentNumber: "RG202600012", DocumentDate: "2026-01-15"}},
		{ParticipantId: withoutFee.Id.String(), EnergyBilled: true},
		// a participant of another tenant: ignored
		{ParticipantId: "ea9942da-03da-11ee-b82b-5a985b4b033a", EnergyBilled: true,
			JoiningFee: &model.JoiningFeeDocument{DocumentNumber: "RG-FREMD", DocumentDate: "2026-02-01"}},
	}

	t.Run("with the list from billing", func(t *testing.T) {
		buf, err := ExportMasterdataToExcel(participants, eeg, tariffNames, joiningFees, states)
		require.NoError(t, err)
		rows := exportRows(t, buf)
		require.Len(t, rows, 4, "header + one row per metering point")
		tariffCol := column(t, rows[0], "Mitgliedstarif")
		assert.Equal(t, "Beitrittsgebühr (€)", rows[0][tariffCol+1])
		assert.Equal(t, "Beitrittsgebühr verrechnet", rows[0][tariffCol+2])
		assert.Equal(t, "Zählpunkt", rows[0][tariffCol+3])

		for _, row := range rows[1:3] { // every row of the member
			assert.Equal(t, "Beitrag mit Gebühr", row[tariffCol])
			assert.Equal(t, "49.9", row[tariffCol+1])
			assert.Equal(t, "RG202600012 vom 15.01.2026", row[tariffCol+2])
		}
		assert.Equal(t, "Beitrag", rows[3][tariffCol])
		assert.Equal(t, "", rows[3][tariffCol+1], "no fee: empty")
		assert.Equal(t, "", rows[3][tariffCol+2], "not billed: empty")
		assert.Equal(t, "AT0030000000000000000000000000003", rows[3][tariffCol+3])
		for _, row := range rows {
			for _, cell := range row {
				assert.NotContains(t, cell, "RG-FREMD")
			}
		}
	})

	t.Run("without a list", func(t *testing.T) {
		buf, err := ExportMasterdataToExcel(participants, eeg, tariffNames, joiningFees, nil)
		require.NoError(t, err)
		rows := exportRows(t, buf)
		require.Len(t, rows, 4)
		tariffCol := column(t, rows[0], "Mitgliedstarif")
		for _, row := range rows[1:3] {
			assert.Equal(t, "49.9", row[tariffCol+1])
			assert.Equal(t, "", row[tariffCol+2])
		}
	})
}

func Test_joiningFeeBilledLabel(t *testing.T) {
	assert.Equal(t, "", joiningFeeBilledLabel(nil))
	assert.Equal(t, "RG202600012 vom 15.01.2026",
		joiningFeeBilledLabel(&model.JoiningFeeDocument{DocumentNumber: "RG202600012", DocumentDate: "2026-01-15"}))
	// a date in another format is shown as sent
	assert.Equal(t, "RG1 vom 15.1.2026", joiningFeeBilledLabel(&model.JoiningFeeDocument{DocumentNumber: "RG1", DocumentDate: "15.1.2026"}))
}
