package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 261005-ca1: PUT /participant/{id} updates the member named in the path. A body that
// carries another id is refused before any write.
func TestUpdateParticipantPathIdMustMatchBody(t *testing.T) {
	const (
		tenant  = "TE000006"
		pathId  = "ea1142dc-03da-11ee-b82b-5a985b4b0306"
		otherId = "ea1142dc-03da-11ee-b82b-5a985b4b0316" // same community
	)
	db, err := database.GetDB(context.Background())
	require.NoError(t, err)
	h := NewParticipantHandler(db)
	claims := &middleware.PlatformClaims{Username: "test"}

	firstNames := func() []string {
		names := []string{}
		for _, id := range []string{pathId, otherId} {
			p, err := db.GetParticipant(context.Background(), tenant, id)
			require.NoError(t, err)
			names = append(names, p.FirstName)
		}
		return names
	}
	before := firstNames()

	for _, tc := range []struct{ name, path, body string }{
		{"other id in body", pathId, `{"id":"` + otherId + `","firstname":"CHANGED"}`},
		{"no id in body", pathId, `{"firstname":"CHANGED"}`},
		{"no valid id in path", "x", `{"id":"` + pathId + `","firstname":"CHANGED"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("PUT", "/participant/"+tc.path, strings.NewReader(tc.body))
			req = mux.SetURLVars(req, map[string]string{"id": tc.path})
			w := httptest.NewRecorder()

			h.updateParticipant()(w, req, claims, tenant)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, before, firstNames(), "no member may be changed")
		})
	}
}
