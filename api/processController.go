package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"github.com/gorilla/mux"
	"net/http"
	"strconv"
)

func InitProcessRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/process").Subrouter()

	s.HandleFunc("/history", jwtWrapper(fetchProcessHistory())).Methods("GET")

	return r
}

func fetchProcessHistory() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		start, err := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
		end, err := strconv.ParseInt(r.URL.Query().Get("end"), 10, 64)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		history, err := database.FetchEdaHistory(database.GetDBXConnection, tenant, start, end)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, 200, history)
	}
}
