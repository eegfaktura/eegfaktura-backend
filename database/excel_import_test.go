package database

import (
	"context"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/model"
	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// builds an in-memory workbook that mirrors the current import template
// ("250310-vorlage-import-stammdaten"): marker rows, a header row starting
// with "Netzbetreiber" and data rows. Date cells are written both as
// d.m.yyyy text and as real Excel date serials (RawCellValue mode).
func buildImportSheet(t *testing.T, rows [][]interface{}) *excelize.File {
	t.Helper()
	f := excelize.NewFile()
	const sheet = "EEG Stammdaten"
	_, err := f.NewSheet(sheet)
	require.NoError(t, err)

	header := []interface{}{
		"Netzbetreiber", "Gemeinschafts-ID", "PLZ", "Ort", "Straße", "Hausnummer",
		"Zählpunkt", "Energierichtung", "Name 1", "Name 2", "BusinessRole",
		"Mitglied seit", "email", "MitgliedsNr", "Zählpunktstatus",
		"registriert seit", "Zugeteilte Menge in Prozent",
	}
	require.NoError(t, f.SetSheetRow(sheet, "A1", &[]interface{}{"[### Leerzeile für Importer ###]"}))
	require.NoError(t, f.SetSheetRow(sheet, "A2", &header))
	for i, r := range rows {
		cell, err := excelize.CoordinatesToCellName(1, 3+i)
		require.NoError(t, err)
		require.NoError(t, f.SetSheetRow(sheet, cell, &r))
	}
	return f
}

func transformSheet(t *testing.T, f *excelize.File, online bool) ([]*model.EegParticipant, *model.Log) {
	t.Helper()
	rows, err := f.Rows("EEG Stammdaten")
	require.NoError(t, err)
	defer rows.Close()
	importLog := &model.Log{Operation: "Excel Master Data Import", Messages: []*model.LogMessage{}}
	return transformExcelData(rows, func(id string) string { return id }, online, importLog), importLog
}

// Template column "Mitglied seit" must become the participant's
// participantSince (as text and as Excel date serial); the metering point's
// registeredSince must come from "registriert seit" instead.
func Test_transformExcelData_memberAndRegisteredSince(t *testing.T) {
	const serial20240101 = 45292 // Excel serial for 2024-01-01

	f := buildImportSheet(t, [][]interface{}{
		{"AT009999", "", "4020", "Linz", "Weg", "1",
			"AT0099990000000000000000000000001", "CONSUMPTION", "Alice", "Alpha", "privat",
			"15.6.2023", "alice@example.org", "001", "ACTIVE",
			serial20240101, "50"},
		{"AT009999", "", "4020", "Linz", "Weg", "2",
			"AT0099990000000000000000000000002", "CONSUMPTION", "Bob", "Beta", "privat",
			serial20240101, "bob@example.org", "002", "ACTIVE",
			"1.6.2022", ""},
		{"AT009999", "", "4020", "Linz", "Weg", "3",
			"AT0099990000000000000000000000003", "CONSUMPTION", "Carl", "Gamma", "privat",
			"", "carl@example.org", "003", "ACTIVE",
			"", ""},
	})

	participants, importLog := transformSheet(t, f, false)
	require.Len(t, participants, 3)
	assert.Empty(t, importLog.Messages)

	alice, bob, carl := participants[0], participants[1], participants[2]

	// "Mitglied seit" -> participantSince (text and serial cell)
	require.True(t, alice.ParticipantSince.Valid)
	assert.Equal(t, civil.DateFor(2023, time.June, 15), alice.ParticipantSince.Date)
	require.True(t, bob.ParticipantSince.Valid)
	assert.Equal(t, civil.DateFor(2024, time.January, 1), bob.ParticipantSince.Date)
	// empty -> today
	require.True(t, carl.ParticipantSince.Valid)
	assert.Equal(t, civil.Today(), carl.ParticipantSince.Date)

	// "registriert seit" -> metering point registeredSince (serial and text cell)
	require.Len(t, alice.MeteringPoint, 1)
	assert.Equal(t, civil.DateFor(2024, time.January, 1), alice.MeteringPoint[0].RegisteredSince)
	assert.Equal(t, civil.DateFor(2024, time.January, 1), alice.MeteringPoint[0].State.ActiveSince.Date)
	require.Len(t, bob.MeteringPoint, 1)
	assert.Equal(t, civil.DateFor(2022, time.June, 1), bob.MeteringPoint[0].RegisteredSince)
	// empty -> Jan 1 of the current year
	require.Len(t, carl.MeteringPoint, 1)
	assert.Equal(t, civil.DateFor(time.Now().Year(), time.January, 1), carl.MeteringPoint[0].RegisteredSince)

	// online EEGs keep registeredSince = today, participantSince still from the sheet
	participantsOnline, _ := transformSheet(t, f, true)
	require.Len(t, participantsOnline, 3)
	assert.Equal(t, civil.Today(), participantsOnline[0].MeteringPoint[0].RegisteredSince)
	assert.Equal(t, civil.DateFor(2023, time.June, 15), participantsOnline[0].ParticipantSince.Date)
}

// Template column "Zugeteilte Menge in Prozent" must feed partFact: plain
// numbers, "%"-suffixed text and percent-formatted cells (raw fraction).
func Test_transformExcelData_partFact(t *testing.T) {
	row := func(nr int, name string, partFact interface{}) []interface{} {
		return []interface{}{"AT009999", "", "4020", "Linz", "Weg", "1",
			"AT009999000000000000000000000000" + string(rune('0'+nr)), "CONSUMPTION", name, "Tester", "privat",
			"", name + "@example.org", "00" + string(rune('0'+nr)), "ACTIVE",
			"", partFact}
	}
	f := buildImportSheet(t, [][]interface{}{
		row(1, "Plain", "50"),
		row(2, "Percent", "75%"),
		row(3, "Fraction", 0.5), // Prozent-formatierte Zelle: 50 % -> Rohwert 0.5
		row(4, "Empty", ""),
		row(5, "Comma", "50,5"),
	})

	participants, _ := transformSheet(t, f, false)
	require.Len(t, participants, 5)

	assert.Equal(t, 50, participants[0].MeteringPoint[0].PartFact)
	assert.Equal(t, 75, participants[1].MeteringPoint[0].PartFact)
	assert.Equal(t, 50, participants[2].MeteringPoint[0].PartFact)
	assert.Equal(t, 100, participants[3].MeteringPoint[0].PartFact)
	assert.Equal(t, 51, participants[4].MeteringPoint[0].PartFact)
}

// Silently skipped rows must surface in the import log: data-looking rows with
// an invalid grid operator and rows whose name cannot be extracted. A trailing
// space in the operator column must no longer discard the row.
func Test_transformExcelData_skipReporting(t *testing.T) {
	f := buildImportSheet(t, [][]interface{}{
		{"at009999", "", "4020", "Linz", "Weg", "1",
			"AT0099990000000000000000000000701", "CONSUMPTION", "Lower", "Case", "privat",
			"", "", "010", "ACTIVE", "", ""},
		{"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"AT009999 ", "", "4020", "Linz", "Weg", "1",
			"AT0099990000000000000000000000702", "CONSUMPTION", "Trail", "Space", "privat",
			"", "", "011", "ACTIVE", "", ""},
		{"AT009999", "", "4020", "Linz", "Weg", "1",
			"AT0099990000000000000000000000703", "CONSUMPTION", "", "Einwortname", "privat",
			"", "", "012", "ACTIVE", "", ""},
	})

	participants, importLog := transformSheet(t, f, false)

	// nur die Zeile mit nachgestelltem Leerzeichen wird (jetzt) importiert
	require.Len(t, participants, 1)
	assert.Equal(t, "Trail", participants[0].FirstName)
	assert.Equal(t, "AT009999", participants[0].MeteringPoint[0].GridOperatorId.String)

	// ungültiger Netzbetreiber + Name-Split-Fehler werden gemeldet, die leere Zeile nicht
	require.Len(t, importLog.Messages, 2)
	assert.Equal(t, "E_PARTICIPANT_1002", importLog.Messages[0].MessageCode)
	assert.Equal(t, "E_PARTICIPANT_1001", importLog.Messages[1].MessageCode)
}

// Rows of one member (one row per metering point) merge by first+last name —
// even when the file numbers MitgliedsNr per ROW, as legacy files do (see the
// TE100200 fixture: Finnegan carries 003 and 004). The number must therefore
// not split members; known limitation: real namesakes merge as well.
func Test_transformExcelData_multiRowMemberMerge(t *testing.T) {
	doe := func(nr, zp string) []interface{} {
		return []interface{}{"AT009999", "", "4020", "Linz", "Weg", "1",
			zp, "CONSUMPTION", "John", "Doe", "privat", "", "", nr, "ACTIVE", "", ""}
	}
	f := buildImportSheet(t, [][]interface{}{
		doe("100", "AT0099990000000000000000000000711"),
		doe("101", "AT0099990000000000000000000000712"),
		doe("", "AT0099990000000000000000000000713"),
	})

	participants, _ := transformSheet(t, f, false)
	require.Len(t, participants, 1)
	assert.Equal(t, "100", participants[0].ParticipantNumber.String)
	assert.Len(t, participants[0].MeteringPoint, 3)
}

// End-to-end against the test database: one failing participant (duplicate
// active metering point) must not abort the rest of the import; a re-import
// attaches new metering points to the existing member (matched by name).
func TestImportMasterdataFromExcel_continueOnError(t *testing.T) {
	db, err := GetDB(context.Background())
	require.NoError(t, err)

	// eigener Tenant, damit andere Tests (Zähl-Assertions auf TE000001/TE000002)
	// nicht durch die hier importierten Mitglieder beeinflusst werden
	tenant := "TE000009"
	_, err = testDB.DbInstance.Exec(`INSERT INTO base.eeg
		(tenant, name, description, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
		 street, "streetNumber", city, zip, email)
		VALUES ('TE000009','IMPORT-TEST','Import-Testgemeinschaft','TE000009','LOCAL','AT009999','Test Operator',
		 'AT00999900000TC000009000000000001','Solarweg','1','Linz','4020','import-test@example.org')
		ON CONFLICT (tenant) DO NOTHING`)
	require.NoError(t, err)

	importFile := func(t *testing.T, rows [][]interface{}) {
		f := buildImportSheet(t, rows)
		buf, err := f.WriteToBuffer()
		require.NoError(t, err)
		require.NoError(t, db.ImportMasterdataFromExcel(context.Background(), buf, "test.xlsx", "EEG Stammdaten", tenant))
	}
	dataRow := func(name1, name2, nr, zp string) []interface{} {
		return []interface{}{"AT009999", "", "4020", "Linz", "Weg", "1",
			zp, "CONSUMPTION", name1, name2, "privat", "", "", nr, "ACTIVE", "", ""}
	}
	byName := func(ps []*model.EegParticipant, lastname string) []*model.EegParticipant {
		var out []*model.EegParticipant
		for _, p := range ps {
			if p.LastName == lastname {
				out = append(out, p)
			}
		}
		return out
	}

	// P2 verwendet denselben (aktiven) Zählpunkt wie P1 -> idx_unique_meteringpoint_active
	// schlägt fehl; P1 und P3 müssen trotzdem importiert werden.
	importFile(t, [][]interface{}{
		dataRow("Astrid", "Alba", "801", "AT0099990000000000000000000000801"),
		dataRow("Bernd", "Brix", "802", "AT0099990000000000000000000000801"),
		dataRow("Clara", "Cox", "803", "AT0099990000000000000000000000802"),
	})
	ps, err := db.GetParticipants(context.Background(), tenant)
	require.NoError(t, err)
	assert.Len(t, byName(ps, "Alba"), 1)
	assert.Len(t, byName(ps, "Cox"), 1)
	assert.Empty(t, byName(ps, "Brix"))

	// Re-Import: neue ZP-Zeile eines bestehenden Mitglieds (Name-Match) wird
	// angehängt, es entsteht kein Duplikat.
	importFile(t, [][]interface{}{
		dataRow("Clara", "Cox", "803", "AT0099990000000000000000000000803"),
	})
	ps, err = db.GetParticipants(context.Background(), tenant)
	require.NoError(t, err)
	cox := byName(ps, "Cox")
	require.Len(t, cox, 1)
	assert.Len(t, cox[0].MeteringPoint, 2)
}
