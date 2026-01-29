package api

import (
	"context"
	"net/http"
	"strconv"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/gorilla/mux"
)

func InitProcessRouter(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/process").Subrouter()

	s.HandleFunc("/history", middleware.Protect(fetchProcessHistory())).Methods("GET")

	return r
}

func fetchProcessHistory() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		
		// parse parameters with separate checks
		startStr := r.URL.Query().Get("start")
		if startStr == "" {
			respondWithError(w, http.StatusBadRequest, "missing start")
			return
		}
		start, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid start")
			return
		}

		endStr := r.URL.Query().Get("end")
		end, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid end")
			return
		}

		psStr := r.URL.Query().Get("ps")
		pageSize, err := strconv.ParseInt(psStr, 10, 64)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid page size")
			return
		}

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
