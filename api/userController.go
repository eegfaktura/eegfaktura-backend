package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/gorilla/mux"
	"net/http"
)

func InitUserRouter(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/user").Subrouter()

	s.HandleFunc("get-user", middleware.Protect(getUser())).Methods("GET")

	return r
}

func getUser() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		tenants := claims.Tenants
		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		tn, err := database.FetchTenantsName(db, tenants)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetUser(err))
			return
		}
		respondWithJSON(w, http.StatusOK, tn)
	}
}
