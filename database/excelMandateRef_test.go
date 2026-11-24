package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

// W-261124-mm1: an empty or blank "MandatRef" cell, or no such column, is stored as NULL (base.billing_masterdata
// then uses the member number); a value is taken without surrounding blanks.
func Test_transformExcelData_mandateReference(t *testing.T) {
	row := func(nr int, mandateRef string) []interface{} {
		r := debitRow(nr, fmt.Sprintf("Mandat%d", nr), "CORE")
		return append(r, mandateRef)
	}
	f := buildImportSheetWithColumns(t, []interface{}{"Einzugsart", "MandatRef"}, [][]interface{}{
		row(1, ""), row(2, "   "), row(3, " MR-0815 "),
	})
	participants, importLog := transformSheet(t, f, false)
	require.Len(t, participants, 3)
	assert.Empty(t, importLog.Messages)
	assert.Equal(t, null.String{}, participants[0].BankAccount.MandateReference)
	assert.Equal(t, null.String{}, participants[1].BankAccount.MandateReference)
	assert.Equal(t, null.StringFrom("MR-0815"), participants[2].BankAccount.MandateReference)

	// without the column
	f = buildImportSheetWithColumns(t, []interface{}{"Einzugsart"}, [][]interface{}{debitRow(1, "Ohne", "CORE")})
	participants, _ = transformSheet(t, f, false)
	require.Len(t, participants, 1)
	assert.Equal(t, null.String{}, participants[0].BankAccount.MandateReference)
}

// A blank "UmsatzsteuerNr" is stored as NULL: billing would take " " as a VAT number (producer gets a reverse charge
// or info document instead of a credit note); a value is taken without surrounding blanks.
func Test_transformExcelData_vatNumber(t *testing.T) {
	row := func(nr int, vat string) []interface{} {
		return append(debitRow(nr, fmt.Sprintf("Ust%d", nr), "CORE"), vat)
	}
	f := buildImportSheetWithColumns(t, []interface{}{"Einzugsart", "UmsatzsteuerNr"}, [][]interface{}{
		row(1, ""), row(2, "  "), row(3, " ATU12345678 "),
	})
	participants, _ := transformSheet(t, f, false)
	require.Len(t, participants, 3)
	assert.Equal(t, null.String{}, participants[0].VatNumber)
	assert.Equal(t, null.String{}, participants[1].VatNumber)
	assert.Equal(t, null.StringFrom("ATU12345678"), participants[2].VatNumber)
}

// End to end: a member imported with a blank "MandatRef" gets the member number as mandate reference in
// base.billing_masterdata (the contract with billing), one with a value keeps it.
func TestImportMasterdataFromExcel_mandateReferenceNull(t *testing.T) {
	const tenant, communityId = "TE000268", "AT00999900000TC000268000000000001"
	createImportTenant(t, tenant, communityId)
	db, err := GetDB(context.Background())
	require.NoError(t, err)

	row := func(name, nr, zp, mandateRef string) []interface{} {
		return []interface{}{"AT009999", communityId, "4020", "Linz", "Weg", "1", zp, "CONSUMPTION", name, "Mandat",
			"privat", "", "", nr, "ACTIVE", "", "100", mandateRef}
	}
	f := buildImportSheetWithColumns(t, []interface{}{"MandatRef"}, [][]interface{}{
		row("Leer", "M-101", "AT0099990000000000000000000268101", "  "),
		row("Voll", "M-102", "AT0099990000000000000000000268102", "MR-77"),
	})
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	require.NoError(t, db.ImportMasterdataFromExcel(context.Background(), buf, "test.xlsx", "EEG Stammdaten", tenant))

	mandateRef := func(firstname string) (null.String, string) {
		var stored null.String
		var billing string
		require.NoError(t, testDB.DbInstance.QueryRow(`SELECT b.mandate_reference, m.participant_sepa_mandate_reference
			FROM base.participant p JOIN base.bankaccount b ON b.participant_id = p.id
			JOIN base.billing_masterdata m ON m.participant_id = p.id
			WHERE p.tenant = $1 AND p.firstname = $2 AND p.lastname = 'Mandat' LIMIT 1`, tenant, firstname).Scan(&stored, &billing))
		return stored, billing
	}
	stored, billing := mandateRef("Leer")
	assert.False(t, stored.Valid, "a blank cell is stored as NULL")
	assert.Equal(t, "M-101", billing, "billing falls back to the member number")
	stored, billing = mandateRef("Voll")
	assert.Equal(t, null.StringFrom("MR-77"), stored)
	assert.Equal(t, "MR-77", billing)
}
