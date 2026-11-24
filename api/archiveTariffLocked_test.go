package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type archiveResult struct {
	database.Database
	err error
}

func (a archiveResult) ArchiveTariff(context.Context, string, string) error { return a.err }

// Archiving a tariff while a bulk change of the community runs too long: 409 with the text; other failures stay 400.
func TestArchiveTariff_lockedIsConflict(t *testing.T) {
	for _, c := range []struct {
		err  error
		want int
	}{
		{model.ErrTariffLocked(errors.New("Gerade läuft eine Massenänderung, bitte später erneut versuchen.")), http.StatusConflict},
		{model.ErrTariffUtilized(errors.New("in use")), http.StatusBadRequest},
	} {
		h := NewEegHandler(archiveResult{err: c.err})
		req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/eeg/tariff/x", nil), map[string]string{"id": "x"})
		w := httptest.NewRecorder()
		h.archiveTariff()(w, req, &middleware.PlatformClaims{Username: "admin"}, "TE000001")
		require.Equal(t, c.want, w.Code)
		assert.Contains(t, w.Body.String(), model.UserText(c.err))
	}
}
