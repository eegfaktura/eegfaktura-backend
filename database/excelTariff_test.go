package database

import (
	"context"
	"encoding/json"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gopkg.in/guregu/null.v4"
)

const (
	tariffMember  = "5e0c7a10-0000-4c55-9a51-7d3f2b6a0001"
	tariffConsume = "5e0c7a10-0000-4c55-9a51-7d3f2b6a0002"
	tariffProduce = "5e0c7a10-0000-4c55-9a51-7d3f2b6a0003"
	tariffTwinA   = "5e0c7a10-0000-4c55-9a51-7d3f2b6a0004"
	tariffTwinB   = "5e0c7a10-0000-4c55-9a51-7d3f2b6a0005"
	tariffOther   = "5e0c7a10-0000-4c55-9a51-7d3f2b6a0006"
)

// the active tariffs of the transform tests; "Standard" exists as member and consumption tariff, "Doppelt" twice
func testImportTariffs() *importTariffs {
	tariff := func(id string, typ model.TariffModelType, name string) model.Tariff {
		return model.Tariff{Id: uuid.Parse(id), Type: typ, Name: name}
	}
	return newImportTariffs([]model.Tariff{
		tariff(tariffMember, model.EEG, "Standard"),
		tariff(tariffConsume, model.VZP, " Standard "),
		tariff(tariffProduce, model.EZP, "Einspeisung"),
		tariff(tariffTwinA, model.VZP, "Doppelt"),
		tariff(tariffTwinB, model.VZP, "doppelt"),
		tariff(tariffOther, model.EZP, "Doppelt"),
	})
}

// tariffRow is a data row with the extra columns "Mitgliedstarif" and "Zp. Tarifname".
func tariffRow(nr int, name, direction, memberTariff, meterTariff string) []interface{} {
	n := string(rune('0' + nr))
	return []interface{}{"AT009999", testCommunityId, "4020", "Linz", "Weg", "1",
		"AT00999900000000000000000000002" + "0" + n, direction, name, "Tarif", "privat",
		"", "", "00" + n, "ACTIVE", "", "100", memberTariff, meterTariff}
}

func transformTariffRows(t *testing.T, rows ...[]interface{}) ([]*model.EegParticipant, *model.Log) {
	t.Helper()
	return transformSheetWithTariffs(t, buildImportSheetWithColumns(t, []interface{}{"Mitgliedstarif", "Zp. Tarifname"}, rows))
}

func transformSheetWithTariffs(t *testing.T, f *excelize.File) ([]*model.EegParticipant, *model.Log) {
	t.Helper()
	sheetRows, err := f.Rows("EEG Stammdaten")
	require.NoError(t, err)
	defer sheetRows.Close()
	importLog := &model.Log{Messages: []*model.LogMessage{}}
	tariffs := testImportTariffs()
	ps := transformExcelData(context.Background(), sheetRows, func(id string) string { return id }, false, testCommunityId,
		tariffs, importLog)
	// the member tariffs as the import resolves them for a member it creates
	for _, p := range ps {
		tariff, hints := tariffs.memberTariff(p, p.FirstName+" "+p.LastName)
		if tariff != nil {
			p.TariffId = null.StringFrom(tariff.Id.String())
		}
		importLog.Messages = append(importLog.Messages, hints...)
	}
	return ps, importLog
}

func Test_transformExcelData_tariffs(t *testing.T) {
	t.Run("found by name, case-insensitive and trimmed, with the type of the column and direction", func(t *testing.T) {
		ps, importLog := transformTariffRows(t,
			tariffRow(1, "Anna", "CONSUMPTION", " standard ", "STANDARD"),
			tariffRow(2, "Bert", "GENERATION", "Standard", " einspeisung"))
		assert.Empty(t, importLog.Messages)
		require.Len(t, ps, 2)
		assert.Equal(t, null.StringFrom(tariffMember), ps[0].TariffId)
		assert.Equal(t, null.StringFrom(tariffConsume), ps[0].MeteringPoint[0].TariffId)
		assert.Equal(t, null.StringFrom(tariffProduce), ps[1].MeteringPoint[0].TariffId)
	})

	t.Run("empty cells: no tariff, no hint", func(t *testing.T) {
		ps, importLog := transformTariffRows(t, tariffRow(1, "Anna", "CONSUMPTION", "", "  "))
		assert.Empty(t, importLog.Messages)
		require.Len(t, ps, 1)
		assert.False(t, ps[0].TariffId.Valid)
		assert.False(t, ps[0].MeteringPoint[0].TariffId.Valid)
	})

	t.Run("unknown, unsuitable or ambiguous: imported without the tariff, with a hint", func(t *testing.T) {
		ps, importLog := transformTariffRows(t,
			tariffRow(1, "Anna", "CONSUMPTION", "Gibt es nicht", "Einspeisung"),
			tariffRow(2, "Bert", "GENERATION", "Einspeisung", "Standard"),
			tariffRow(3, "Cora", "CONSUMPTION", "Standard", "Doppelt"))
		require.Len(t, ps, 3)
		for _, p := range ps[:2] {
			assert.False(t, p.TariffId.Valid)
		}
		assert.Equal(t, null.StringFrom(tariffMember), ps[2].TariffId)
		for _, p := range ps {
			assert.False(t, p.MeteringPoint[0].TariffId.Valid)
		}
		want := []struct{ identifier, code, column string }{
			{"AT0099990000000000000000000000201", "W_TARIFF_TYPE", importMeterTariffColumn},
			{"AT0099990000000000000000000000202", "W_TARIFF_TYPE", importMeterTariffColumn},
			{"AT0099990000000000000000000000203", "W_TARIFF_AMBIGUOUS", importMeterTariffColumn},
			{"Anna Tarif", "W_TARIFF_UNKNOWN", importMemberTariffColumn},
			{"Bert Tarif", "W_TARIFF_TYPE", importMemberTariffColumn},
		}
		require.Len(t, importLog.Messages, len(want))
		for i, w := range want {
			m := importLog.Messages[i]
			assert.Equal(t, "WARNING", m.Kind)
			assert.Equal(t, w.identifier, m.Identifier)
			assert.Equal(t, w.code, m.MessageCode)
			assert.Contains(t, m.Message, w.column)
		}
	})

	t.Run("a member on several rows: the first non-empty member tariff, a tariff per metering point", func(t *testing.T) {
		row := func(nr int, memberTariff, meterTariff string) []interface{} {
			r := tariffRow(nr, "Same", "CONSUMPTION", memberTariff, meterTariff)
			r[13] = "001"
			return r
		}
		ps, importLog := transformTariffRows(t, row(1, "", "Standard"), row(2, "Standard", ""), row(3, " standard ", ""))
		assert.Empty(t, importLog.Messages, "the same name in another spelling is no conflict")
		require.Len(t, ps, 1)
		assert.Equal(t, null.StringFrom(tariffMember), ps[0].TariffId)
		require.Len(t, ps[0].MeteringPoint, 3)
		assert.Equal(t, null.StringFrom(tariffConsume), ps[0].MeteringPoint[0].TariffId)
		assert.False(t, ps[0].MeteringPoint[1].TariffId.Valid)
	})

	t.Run("a member on several rows with different member tariffs: the first counts, with a hint", func(t *testing.T) {
		row := func(nr int, memberTariff string) []interface{} {
			r := tariffRow(nr, "Same", "CONSUMPTION", memberTariff, "")
			r[13] = "001"
			return r
		}
		ps, importLog := transformTariffRows(t, row(1, "Standard"), row(2, "Premium"))
		require.Len(t, ps, 1)
		assert.Equal(t, null.StringFrom(tariffMember), ps[0].TariffId)
		require.Len(t, importLog.Messages, 1, "the second name is not looked up")
		m := importLog.Messages[0]
		assert.Equal(t, "WARNING", m.Kind)
		assert.Equal(t, "Same Tarif", m.Identifier)
		assert.Equal(t, "W_TARIFF_CONFLICT", m.MessageCode)
		assert.Contains(t, m.Message, importMemberTariffColumn)
	})
}

// One summary hint about new members with a joining fee in their member tariff.
func Test_joiningFeeMessage(t *testing.T) {
	assert.Nil(t, joiningFeeMessage(0, map[string]bool{}))
	m := joiningFeeMessage(3, map[string]bool{"Mitglied": true, "Familie": true})
	require.NotNil(t, m)
	assert.Equal(t, "WARNING", m.Kind)
	assert.Equal(t, "W_TARIFF_JOINING_FEE", m.MessageCode)
	assert.Contains(t, m.Message, "3 new member(s)")
	assert.Contains(t, m.Message, "(Familie, Mitglied)")
}

// Round trip: the tariff columns of the master data export, header and cell as written, are read by the import.
func Test_transformExcelData_tariffsFromExport(t *testing.T) {
	member := &model.EegParticipant{EegParticipantBase: model.EegParticipantBase{
		Id: uuid.Parse("5e0c7a10-1111-4c55-9a51-7d3f2b6a0001"), FirstName: "Export", LastName: "Tarif",
		TariffId: null.StringFrom(tariffMember),
		MeteringPoint: []*model.MeteringPoint{
			{MeteringPoint: "AT0099990000000000000000000000301", Direction: model.CONSUMPTION, TariffId: null.StringFrom(tariffConsume), State: &model.MeterState{}},
			{MeteringPoint: "AT0099990000000000000000000000302", Direction: model.GENERATOR, TariffId: null.StringFrom(tariffProduce), State: &model.MeterState{}},
		},
	}}
	tariffNames := map[string]string{tariffMember: "Standard", tariffConsume: " Standard ", tariffProduce: "Einspeisung"}
	buf, err := ExportMasterdataToExcel(context.Background(), []*model.EegParticipant{member}, &model.Eeg{RcNumber: "RC100999"}, tariffNames, nil, nil)
	require.NoError(t, err)
	exported := exportRows(t, buf)
	require.Len(t, exported, 3)
	memberCol, meterCol := column(t, exported[0], importMemberTariffColumn), column(t, exported[0], importMeterTariffColumn)
	assert.Equal(t, " Standard ", exported[1][meterCol], "the export writes the plain tariff name")

	rows := [][]interface{}{}
	for i, r := range exported[1:] {
		direction := "CONSUMPTION"
		if i == 1 {
			direction = "GENERATION"
		}
		rows = append(rows, []interface{}{"AT009999", testCommunityId, "4020", "Linz", "Weg", "1", r[column(t, exported[0], "Zählpunkt")],
			direction, "Export", "Tarif", "privat", "", "", "001", "ACTIVE", "", "100", r[memberCol], r[meterCol]})
	}
	f := buildImportSheetWithColumns(t, []interface{}{exported[0][memberCol], exported[0][meterCol]}, rows)
	ps, importLog := transformSheetWithTariffs(t, f)
	assert.Empty(t, importLog.Messages)
	require.Len(t, ps, 1)
	assert.Equal(t, null.StringFrom(tariffMember), ps[0].TariffId)
	require.Len(t, ps[0].MeteringPoint, 2)
	assert.Equal(t, null.StringFrom(tariffConsume), ps[0].MeteringPoint[0].TariffId)
	assert.Equal(t, null.StringFrom(tariffProduce), ps[0].MeteringPoint[1].TariffId)
}

// End to end: the import assigns only active tariffs of the community (latest version, not archived); an existing
// member keeps its member tariff, its new metering point gets the metering point tariff.
func TestImportMasterdataFromExcel_tariffs(t *testing.T) {
	const tenant, communityId = "TE000269", "AT00999900000TC000269000000000001"
	const member, other, consume, produce, archived, renamed = "5e0c7a10-2222-4c55-9a51-7d3f2b6a0001",
		"5e0c7a10-2222-4c55-9a51-7d3f2b6a0002", "5e0c7a10-2222-4c55-9a51-7d3f2b6a0003",
		"5e0c7a10-2222-4c55-9a51-7d3f2b6a0004", "5e0c7a10-2222-4c55-9a51-7d3f2b6a0005",
		"5e0c7a10-2222-4c55-9a51-7d3f2b6a0006"
	createImportTenant(t, tenant, communityId)
	_, err := testDB.DbInstance.Exec(`INSERT INTO base.tariff (id, tenant, type, name, version, status, "joiningFee") VALUES
		($1, $7, 'EEG', 'Mitglied', 0, 'ACTIVE', 50), ($2, $7, 'EEG', 'Anders', 0, 'ACTIVE', 0),
		($3, $7, 'VZP', 'Bezug', 0, 'ACTIVE', 0), ($4, $7, 'EZP', 'Einspeisung', 0, 'ACTIVE', 0),
		($5, $7, 'VZP', 'Archiviert', 0, 'ARCHIVED', 0), ($6, $7, 'VZP', 'Alt', 0, 'INACTIVE', 0), ($6, $7, 'VZP', 'Neu', 1, 'ACTIVE', 0)`,
		member, other, consume, produce, archived, renamed, tenant)
	require.NoError(t, err)
	// a tariff of another community is not found
	const foreign = "5e0c7a10-2222-4c55-9a51-7d3f2b6a0007"
	createImportTenant(t, "TE000270", "AT00999900000TC000270000000000001")
	_, err = testDB.DbInstance.Exec(`INSERT INTO base.tariff (id, tenant, type, name, version, status)
		VALUES ($1, 'TE000270', 'VZP', 'Fremd', 0, 'ACTIVE')`, foreign)
	require.NoError(t, err)
	db, err := GetDB(context.Background())
	require.NoError(t, err)

	importRows := func(rows ...[]interface{}) {
		f := buildImportSheetWithColumns(t, []interface{}{"Mitgliedstarif", "Zp. Tarifname"}, rows)
		buf, err := f.WriteToBuffer()
		require.NoError(t, err)
		require.NoError(t, db.ImportMasterdataFromExcel(context.Background(), buf, "test.xlsx", "EEG Stammdaten", tenant))
	}
	row := func(name, zp, direction, memberTariff, meterTariff string) []interface{} {
		return []interface{}{"AT009999", communityId, "4020", "Linz", "Weg", "1", zp, direction, name, "Tarifimport",
			"privat", "", "", "", "ACTIVE", "", "100", memberTariff, meterTariff}
	}
	memberTariff := func(name string) null.String {
		var id null.String
		require.NoError(t, testDB.DbInstance.Get(&id, `SELECT "tariffId" FROM base.participant
			WHERE tenant = $1 AND firstname = $2 AND lastname = 'Tarifimport'`, tenant, name))
		return id
	}
	// the hints of the last import as "identifier code"
	lastHints := func() []string {
		var notification string
		require.NoError(t, testDB.DbInstance.Get(&notification, `SELECT notification FROM base.notification
			WHERE tenant = $1 ORDER BY id DESC LIMIT 1`, tenant))
		var n struct{ Messages []model.LogMessage }
		require.NoError(t, json.Unmarshal([]byte(notification), &n))
		hints := []string{}
		for _, m := range n.Messages {
			hints = append(hints, m.Identifier+" "+m.MessageCode)
		}
		return hints
	}
	meterTariff := func(zp string) null.String {
		var id null.String
		require.NoError(t, testDB.DbInstance.Get(&id, `SELECT tariff_id FROM base.meteringpoint
			WHERE tenant = $1 AND metering_point_id = $2`, tenant, zp))
		return id
	}

	importRows(
		row("Anna", "AT0099990000000000000000000269001", "CONSUMPTION", "mitglied", "Bezug"),
		row("Bert", "AT0099990000000000000000000269002", "GENERATION", "", "Einspeisung"),
		row("Cora", "AT0099990000000000000000000269003", "CONSUMPTION", "Bezug", "Archiviert"),
		row("Cora", "AT0099990000000000000000000269004", "CONSUMPTION", "", "Alt"),
		row("Dora", "AT0099990000000000000000000269005", "CONSUMPTION", "", "neu"),
		row("Emil", "AT0099990000000000000000000269007", "CONSUMPTION", "", "Fremd"))
	assert.Equal(t, null.StringFrom(member), memberTariff("Anna"))
	assert.False(t, memberTariff("Bert").Valid)
	assert.False(t, memberTariff("Cora").Valid, "a consumption tariff is no member tariff")
	assert.Equal(t, null.StringFrom(consume), meterTariff("AT0099990000000000000000000269001"))
	assert.Equal(t, null.StringFrom(produce), meterTariff("AT0099990000000000000000000269002"))
	assert.False(t, meterTariff("AT0099990000000000000000000269003").Valid, "archived")
	assert.False(t, meterTariff("AT0099990000000000000000000269004").Valid, "an older version's name")
	assert.Equal(t, null.StringFrom(renamed), meterTariff("AT0099990000000000000000000269005"))
	assert.False(t, meterTariff("AT0099990000000000000000000269007").Valid, "another community's tariff")
	assert.Contains(t, lastHints(), "Cora Tarifimport W_TARIFF_TYPE")
	assert.Contains(t, lastHints(), "Mitgliedstarif W_TARIFF_JOINING_FEE", "Anna got a member tariff with a joining fee")

	var feeName string
	require.NoError(t, testDB.DbInstance.Get(&feeName, `SELECT tariff_participant_fee_name FROM base.billing_masterdata
		WHERE participant_firstname = 'Anna' AND participant_lastname = 'Tarifimport' AND metering_point_id = $1`,
		"AT0099990000000000000000000269001"))
	assert.Equal(t, "Mitglied", feeName, "billing sees the member tariff")

	// re-import: Anna exists, gets the new metering point with its tariff and keeps her member tariff; the ignored
	// member tariff cell of an existing member is reported as such, an unknown name in it is not looked up
	importRows(row("Anna", "AT0099990000000000000000000269006", "CONSUMPTION", "Anders", "Bezug"),
		row("Bert", "AT0099990000000000000000000269008", "GENERATION", "Unbekannt", ""))
	assert.Equal(t, null.StringFrom(member), memberTariff("Anna"))
	assert.Equal(t, null.StringFrom(consume), meterTariff("AT0099990000000000000000000269006"))
	assert.False(t, memberTariff("Bert").Valid)
	assert.ElementsMatch(t, []string{"Anna Tarifimport W_TARIFF_MEMBER_EXISTS", "Bert Tarifimport W_TARIFF_MEMBER_EXISTS"}, lastHints())
}
