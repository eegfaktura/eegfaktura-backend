package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"github.com/gorilla/mux"
)

// maxHistoryPageSize is the largest page size of /process/history; larger values are
// clamped (callers page via "next"). A page size of 0 means "no paging" (used by eegfaktura-web).
const maxHistoryPageSize = 1000

// parseHistoryPageSize parses the ps query parameter: a non-negative integer, clamped to
// maxHistoryPageSize. Negative and non-numeric values are an error.
func parseHistoryPageSize(ps string) (uint, error) {
	// On overflow ParseInt returns the int64 bound with the sign of the input.
	pageSize, err := strconv.ParseInt(ps, 10, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, err
	}
	if pageSize < 0 {
		return 0, fmt.Errorf("negative page size: %s", ps)
	}
	if pageSize > maxHistoryPageSize {
		return maxHistoryPageSize, nil
	}
	return uint(pageSize), nil
}

func InitProcessRouter(r *mux.Router, db database.Database) *mux.Router {
	h := NewProcessHandler(db)
	s := r.PathPrefix("/process").Subrouter()

	s.HandleFunc("/history", middleware.Protect(h.fetchProcessHistory())).Methods("GET")

	return r
}

type ProcessHandler struct {
	db database.Database
}

func NewProcessHandler(db database.Database) *ProcessHandler {
	return &ProcessHandler{db: db}
}

func (h *ProcessHandler) fetchProcessHistory() middleware.JWTHandlerFunc {
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

		pageSize, err := parseHistoryPageSize(r.URL.Query().Get("ps"))
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid page size")
			return
		}

		protocol := r.URL.Query().Get("protocol")

		history, err := h.db.FetchEdaHistory(tenant, protocol, start, end, pageSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithData(w, 200, history)
	}
}
