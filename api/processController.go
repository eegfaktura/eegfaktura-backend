package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"context"
	"github.com/gorilla/mux"
	"net/http"
	"strconv"
)

func InitProcessRouter(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/process").Subrouter()

	s.HandleFunc("/history", middleware.Protect(fetchProcessHistory())).Methods("GET")

	return r
}

func fetchProcessHistory() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		start, err := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
		end, err := strconv.ParseInt(r.URL.Query().Get("end"), 10, 64)
		pageSize, err := strconv.ParseInt(r.URL.Query().Get("ps"), 10, 16)
		protocol := r.URL.Query().Get("protocol")
		if err != nil {
			respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		db, err := database.GetDB(context.Background())
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		history, err := db.FetchEdaHistory(tenant, protocol, start, end, uint(pageSize))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, 200, history)
	}
}
