package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	log "github.com/sirupsen/logrus"
)

// settlementIntervalRequest is the body of POST /eeg/settlementinterval (konzept-abrechnungsintervall.md): web passes
// billing's settlementState on as billingState.
type settlementIntervalRequest struct {
	SettlementInterval string              `json:"settlementInterval"`
	DryRun             bool                `json:"dryRun"`
	BillingState       *model.BillingState `json:"billingState"`
}

type settlementIntervalResponse struct {
	model.SettlementIntervalDecision
	Eeg *model.Eeg `json:"eeg"`
}

// maxSettlementIntervalBytes bounds the body: two lists of clearing period identifiers.
const maxSettlementIntervalBytes = 1 << 20

// changeSettlementInterval checks a change of the settlement interval against the rule and, without dryRun, stores
// it with its record. 200 with the decision (allowed false only with dryRun), 409 when the rule blocks a change, 400
// for an invalid request or the current interval.
func (h *EegHandler) changeSettlementInterval() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		var req settlementIntervalRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSettlementIntervalBytes))
		if err := dec.Decode(&req); err != nil {
			respondWith(r.Context(), w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}
		// nothing after the JSON body but white space (eegfaktura-dev#54 #9)
		if err := dec.Decode(&json.RawMessage{}); err != io.EOF {
			respondWith(r.Context(), w, http.StatusBadRequest, tenant, model.ErrParseJson(errors.New("data after the JSON body")))
			return
		}
		if !model.IsSettlementInterval(req.SettlementInterval) {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1060,
				"Ungültiges Abrechnungsintervall (erlaubt: MONTHLY, QUARTER, BIANNUAL, ANNUAL)"))
			return
		}
		if err := model.CheckBillingState(req.BillingState); err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1060, err.Error()))
			return
		}

		decision, err := h.db.ChangeSettlementInterval(r.Context(), tenant, req.SettlementInterval, claims.Username,
			*req.BillingState, req.DryRun)
		switch {
		case errors.Is(err, database.ErrSettlementIntervalUnchanged):
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1061, "Das Abrechnungsintervall ist bereits eingestellt"))
			return
		case errors.Is(err, database.ErrSettlementIntervalNoEeg):
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1001, "Die Gemeinschaft ist nicht angelegt"))
			return
		case err != nil:
			// no database text to the client
			log.WithContext(r.Context()).WithError(err).Error("settlement interval change failed")
			respondWithHttpError(w, http.StatusInternalServerError, BadProcessError(1063, "Das Abrechnungsintervall konnte nicht geändert werden"))
			return
		case !decision.Allowed && !req.DryRun:
			respondWithHttpError(w, http.StatusConflict, BadProcessError(1062, decision.Message))
			return
		}

		resp := settlementIntervalResponse{SettlementIntervalDecision: decision}
		if !req.DryRun {
			if resp.Eeg, err = h.db.GetEegById(r.Context(), tenant); err != nil {
				log.WithContext(r.Context()).WithError(err).Error("settlement interval changed, reading the EEG failed")
				respondWithHttpError(w, http.StatusInternalServerError, BadProcessError(1063, "Das Abrechnungsintervall wurde geändert, die Gemeinschaft konnte nicht neu geladen werden"))
				return
			}
		}
		respondWithJSON(w, http.StatusOK, resp)
	}
}
