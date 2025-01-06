package api

import (
	"net/http"

	"github.com/eegfaktura/eegfaktura-backend/api/middleware"
	"github.com/eegfaktura/eegfaktura-backend/database"
	"github.com/gorilla/mux"
)

func InitProcessRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/process").Subrouter()

	s.HandleFunc("/history", jwtWrapper(fetchProcessHistory())).Methods("GET")

	return r
}

func fetchProcessHistory() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		history, err := database.FetchEdaHistory(database.GetDBXConnection, tenant)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, 200, history)
	}
}
