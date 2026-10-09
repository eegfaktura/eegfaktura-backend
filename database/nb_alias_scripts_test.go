package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#107: scripts/nb-alias-107/check.sql finds wrong stored grid operators, fix.sql corrects
// them (alias list, name fallback to the EEG) and leaves excluded metering points alone.
func TestNbAliasScripts(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)

	const tenant = "TE000010"
	const communityId = "AT00800000000TC000010000000000001"
	_, err = testDB.DbInstance.Exec(`INSERT INTO base.eeg
		(tenant, name, description, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
		 street, "streetNumber", city, zip, email)
		VALUES ('TE000010','NB-ALIAS-TEST','Testgemeinschaft #107','TE000010','LOCAL','AT008000','Energienetze Steiermark',
		 '` + communityId + `','Weg','1','Weiz','8160','nb-alias-test@example.org')
		ON CONFLICT (tenant) DO NOTHING`)
	require.NoError(t, err)

	row := func(name, zp string) []interface{} {
		return []interface{}{"", communityId, "8160", "Weiz", "Weg", "1",
			zp, "CONSUMPTION", name, "NbAlias", "privat", "", "", "", "ACTIVE", "", ""}
	}
	f := buildImportSheet(t, [][]interface{}{
		row("Alt", "AT0082000816000000000000000107001"),
		row("Eigen", "AT0080000816000000000000000107002"),
		row("Zurueck", "AT0082000816000000000000000107003"),
	})
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	require.NoError(t, db.ImportMasterdataFromExcel(ctx, buf, "test.xlsx", "EEG Stammdaten", tenant))

	// simulate the state before #107: old grid operator numbers stored
	_, err = testDB.DbInstance.Exec(`UPDATE base.meteringpoint SET grid_operator_id = 'AT008200', grid_operator_name = 'alt'
		WHERE tenant = 'TE000010' AND metering_point_id IN ('AT0082000816000000000000000107001', 'AT0082000816000000000000000107003')`)
	require.NoError(t, err)
	_, err = testDB.DbInstance.Exec(`UPDATE base.meteringpoint SET grid_operator_id = NULL
		WHERE tenant = 'TE000010' AND metering_point_id = 'AT0080000816000000000000000107002'`)
	require.NoError(t, err)

	// fix.sql corrects ALL tenants: run everything in one transaction and roll it back, so the
	// fixture tenants of other tests stay untouched
	tx, err := testDB.DbInstance.Beginx()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	check := func() []string {
		sql, err := os.ReadFile("../scripts/nb-alias-107/check.sql")
		require.NoError(t, err)
		rows, err := tx.Queryx(string(sql))
		require.NoError(t, err)
		defer rows.Close()
		var found []string
		for rows.Next() {
			r := map[string]interface{}{}
			require.NoError(t, rows.MapScan(r))
			if r["tenant"] == tenant {
				found = append(found, r["metering_point_id"].(string))
			}
		}
		return found
	}
	assert.ElementsMatch(t, []string{
		"AT0082000816000000000000000107001", "AT0080000816000000000000000107002", "AT0082000816000000000000000107003",
	}, check())

	fix, err := os.ReadFile("../scripts/nb-alias-107/fix.sql")
	require.NoError(t, err)
	stmt := strings.Replace(string(fix), "BEGIN;", "", 1)

	// the script as shipped (empty exclusion list) must run as well
	_, err = tx.Exec("SAVEPOINT shipped")
	require.NoError(t, err)
	_, err = tx.Exec(stmt)
	require.NoError(t, err)
	_, err = tx.Exec("ROLLBACK TO SAVEPOINT shipped")
	require.NoError(t, err)

	stmt = strings.Replace(stmt, "(NULL::varchar)  -- e.g. ('AT0040000530000000000000005026510'),",
		"(NULL::varchar), ('AT0082000816000000000000000107003')", 1)
	_, err = tx.Exec(stmt)
	require.NoError(t, err)

	assert.Equal(t, []string{"AT0082000816000000000000000107003"}, check())

	var id, name string
	require.NoError(t, tx.QueryRowx(`SELECT grid_operator_id, grid_operator_name FROM base.meteringpoint
		WHERE tenant = 'TE000010' AND metering_point_id = 'AT0082000816000000000000000107001'`).Scan(&id, &name))
	assert.Equal(t, "AT008000", id)
	assert.NotEqual(t, "alt", name)
}
