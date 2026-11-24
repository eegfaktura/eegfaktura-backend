package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// Bulk change of the support (konzept-massenaenderungen.md): superuser-only routes, called by eegfaktura-admin
// from the browser. The community comes from the header tenant; the body holds the rule (and the hits for apply).

// bulkBodyLimit bounds the request body (apply carries one entry per changed hit): 1 MiB holds about 6,500 hits
// of a member rule with 35-character references and about 4,800 of the metering point tariff.
const bulkBodyLimit = 1 << 20

type BulkChangeHandler struct {
	db database.Database
}

func InitBulkChangeRouter(r *mux.Router, db database.Database) *mux.Router {
	h := &BulkChangeHandler{db: db}
	s := r.PathPrefix("/bulkchange").Subrouter()
	s.HandleFunc("/communities", middleware.ProtectSuperuser(h.communities())).Methods("GET")
	s.HandleFunc("/tariffs", middleware.ProtectSuperuser(h.tariffs())).Methods("GET")
	s.HandleFunc("/preview", middleware.ProtectSuperuser(h.preview())).Methods("POST")
	s.HandleFunc("/apply", middleware.ProtectSuperuser(h.apply())).Methods("POST")
	return r
}

// bulkStatus maps the error codes of the bulk change to the HTTP status.
func bulkStatus(code int) int {
	switch code {
	case 6100, 6101, 6102:
		return http.StatusBadRequest
	case 6103:
		return http.StatusNotFound
	case 6106:
		return http.StatusForbidden
	case 6107:
		return http.StatusRequestEntityTooLarge
	case 6104:
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// respondBulkError answers with the code and German text; a database failure only with a neutral text.
func respondBulkError(w http.ResponseWriter, r *http.Request, tenant string, err error) {
	var ve *model.VfeegError
	if !errors.As(err, &ve) {
		ve = model.ErrBulkChange(err)
	}
	status := bulkStatus(ve.Code)
	if status != http.StatusInternalServerError {
		respondWith(r.Context(), w, status, tenant, ve)
		return
	}
	// logged once with the real error; the client gets a neutral text (respondWith would log a second time)
	log.WithContext(r.Context()).WithField("tenant", tenant).WithField("code", ve.Code).WithError(err).Error("bulk change failed")
	text := "Die Massenänderung ist fehlgeschlagen; es wurde nichts geändert."
	if errors.Is(ve.Err, database.ErrBulkOutcomeUnknown) {
		text = "Das Ergebnis der Massenänderung ist unklar. Bitte die Vorschau neu laden und prüfen."
	}
	respondWithHttpError(w, status, &HttpError{Code: ve.Code, Error: text, Message: text})
}

// decodeBulk reads the body strictly: limited size, no unknown keys, one JSON value.
func decodeBulk(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, bulkBodyLimit))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return model.ErrBulkTooLarge(errors.New("Die Anfrage ist zu groß (höchstens 1 MiB, etwa 5.000 Treffer). " +
			"Bitte die Regel mit einer Bedingung enger fassen und in mehreren Läufen ändern."))
	}
	if err != nil || dec.More() {
		return model.ErrBulkRule(errors.New("Anfrage ungültig"))
	}
	return nil
}

func (h *BulkChangeHandler) communities() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, _ *middleware.PlatformClaims, tenant string) {
		list, err := h.db.BulkCommunities(r.Context())
		if err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusOK, list)
	}
}

// tariffs: the tariffs admin offers for the tariff rules of the community in the header.
func (h *BulkChangeHandler) tariffs() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, _ *middleware.PlatformClaims, tenant string) {
		eeg, err := h.db.BulkEeg(r.Context(), tenant)
		if err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		list, err := h.db.BulkTariffs(r.Context(), eeg)
		if err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusOK, list)
	}
}

func (h *BulkChangeHandler) preview() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, _ *middleware.PlatformClaims, tenant string) {
		eeg, err := h.db.BulkEeg(r.Context(), tenant)
		if err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		var req model.BulkPreviewRequest
		if err := decodeBulk(w, r, &req); err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		if req.Rule == nil {
			respondBulkError(w, r, tenant, model.ErrBulkRule(errors.New("Regel fehlt")))
			return
		}
		if err := req.Rule.Validate(); err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		plan, err := h.db.BulkPreview(r.Context(), eeg, req.Rule)
		if err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusOK, plan)
	}
}

func (h *BulkChangeHandler) apply() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		// The token's user goes into lastModifiedBy/modifiedBy, the only trace of the run. Without it the token is
		// not enough for a write: 403 (it is the token, not the request, that is incomplete).
		if strings.TrimSpace(claims.Username) == "" {
			respondBulkError(w, r, tenant, model.ErrBulkNoUser(errors.New("Im Anmelde-Token fehlt der Benutzername; "+
				"die Massenänderung wird nicht ausgeführt.")))
			return
		}
		eeg, err := h.db.BulkEeg(r.Context(), tenant)
		if err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		var req model.BulkApplyRequest
		if err := decodeBulk(w, r, &req); err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		if req.Rule == nil {
			respondBulkError(w, r, tenant, model.ErrBulkRule(errors.New("Regel fehlt")))
			return
		}
		if err := req.Rule.Validate(); err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		if err := req.ValidateItems(); err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		res, err := h.db.BulkApply(r.Context(), eeg, claims.Username, &req)
		if err != nil {
			respondBulkError(w, r, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusOK, res)
	}
}
