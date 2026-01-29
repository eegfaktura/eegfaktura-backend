package api

import (
	"context"
	"net/http"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/gorilla/mux"
)

func InitUserRouter(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/user").Subrouter()

	s.HandleFunc("/get-user", middleware.Protect(getUser())).Methods("GET")

	return r
}

func getUser() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		//tenants := claims.Tenants
		db, err := database.GetDB(context.Background())
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		var tenants []string
		su := middleware.IsSuperuser(claims.RealmAccess.Roles)
		if !su {
			tenants = claims.Tenants
		}

		tn, err := db.FetchTenantsName(tenants, su)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetUser(err))
			return
		}
		respondWithJSON(w, http.StatusOK, tn)
	}
}
