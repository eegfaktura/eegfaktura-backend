package database

import (
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
