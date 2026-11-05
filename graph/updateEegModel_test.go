package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/database/dbmock"
	"at.ourproject/vfeeg-backend/graph/gmodel"
	"at.ourproject/vfeeg-backend/model"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateEegModel calls the resolver behind GQLProtect, so the tenant comes from the checked header.
func updateEegModel(t *testing.T, arg string, in *gmodel.EegModel) (*model.Eeg, error) {
	t.Helper()
	orig := middleware.VerifyTokenClaims
	t.Cleanup(func() { middleware.VerifyTokenClaims = orig })
	middleware.VerifyTokenClaims = func(context.Context, string) (*middleware.PlatformClaims, error) {
		return &middleware.PlatformClaims{Tenants: []string{"te000233"}, AccessGroups: middleware.AccessGroups{"/EEG_ADMIN"}}, nil
	}
	var eeg *model.Eeg
	var err error
	handler := middleware.GQLProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eeg, err = (&mutationResolver{&Resolver{}}).UpdateEegModel(r.Context(), arg, in)
	}))
	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.Header.Set("Authorization", middleware.BEARER_SCHEMA+"token")
	req.Header.Set("tenant", "TE000233")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return eeg, err
}

// 261105-mm2: updateEegModel stores sepaActiv through the guarded update of POST /eeg for the tenant of the request
// (not the argument), drops settlementInterval and returns the EEG read back.
func TestUpdateEegModelStores(t *testing.T) {
	db, mock, err := dbmock.New()
	require.NoError(t, err)
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(nil) })

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "base"."eeg" SET "sepa"=TRUE WHERE ("tenant" = 'TE000233')`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "base"."eeg" WHERE \("tenant" = \$1\)`).WithArgs("TE000233").
		WillReturnRows(sqlmock.NewRows([]string{"tenant", "settlementInterval", "sepa"}).AddRow("TE000233", "MONTHLY", true))

	interval, sepa := "ANNUAL", true
	eeg, err := updateEegModel(t, "OTHER", &gmodel.EegModel{SettlementInterval: &interval, SepaActiv: &sepa})
	require.NoError(t, err)
	assert.Equal(t, "TE000233", eeg.Id)
	assert.True(t, eeg.Sepa)
	assert.Equal(t, "MONTHLY", eeg.SettlementInterval)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Only settlementInterval (or nothing): no UPDATE, the stored EEG comes back.
func TestUpdateEegModelOnlySettlementInterval(t *testing.T) {
	db, mock, err := dbmock.New()
	require.NoError(t, err)
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(nil) })

	for _, in := range []*gmodel.EegModel{nil, {SettlementInterval: new(string)}} {
		mock.ExpectQuery(`SELECT .* FROM "base"."eeg"`).WithArgs("TE000233").
			WillReturnRows(sqlmock.NewRows([]string{"tenant", "settlementInterval"}).AddRow("TE000233", "MONTHLY"))
		eeg, err := updateEegModel(t, "TE000233", in)
		require.NoError(t, err)
		assert.Equal(t, "MONTHLY", eeg.SettlementInterval)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}
