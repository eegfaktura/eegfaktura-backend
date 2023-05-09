package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"encoding/json"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"time"
)

func InitEegRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/eeg").Subrouter()

	s.HandleFunc("", jwtWrapper(getEEG())).Methods("GET")
	s.HandleFunc("", jwtWrapper(updateEEG())).Methods("POST")
	s.HandleFunc("/tariff", jwtWrapper(getTariff())).Methods("GET")
	s.HandleFunc("/tariff", jwtWrapper(addTariff())).Methods("POST")
	s.HandleFunc("/sync/participants", jwtWrapper(syncParticipantsEda())).Methods("POST")
	s.HandleFunc("/sync/meterpoint", jwtWrapper(syncMeterpointEda())).Methods("POST")
	s.HandleFunc("/import/masterdata", jwtWrapper(uploadMasterData())).Methods("POST")

	return r
}

func getEEG() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		log.Infof("Query EEG with TENANT: %s", tenant)
		eeg, err := database.GetEeg(tenant)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, 200, eeg)
	}
}

func updateEEG() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		var e model.Eeg
		err := json.NewDecoder(r.Body).Decode(&e)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err = database.UpdateEeg(tenant, &e); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, 200, e)
	}
}

func getTariff() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		tariff, err := database.GetTariff(tenant)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, 200, tariff)
	}
}

func addTariff() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		// Try to decode the request body into the struct. If there is an error,
		// respond to the client with the error message and a 400 status code.
		var t model.Tariff
		err := json.NewDecoder(r.Body).Decode(&t)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("ADD TARIF: %+v Tenant: %+v", t, tenant)

		if err = database.AddTariff(tenant, &t); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, http.StatusCreated, t)
	}
}

func syncParticipantsEda() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		eeg, err := database.GetEeg(tenant)
		if err != nil {
			log.WithField("error", err).Error("Query EEG")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		day := time.Now()
		ebmsMessage := model.EbmsMessage{
			Sender:      strings.ToUpper(tenant),
			Receiver:    strings.ToUpper(eeg.GridOperator),
			MessageCode: model.EBMS_ZP_LIST,
			Meter:       &model.Meter{MeteringPoint: eeg.CommunityId},
			Timeline: &model.Timeline{
				From: time.Date(day.Year(), day.Month(), day.Day()-1, 0, 0, 0, 0, day.Location()).UnixMilli(),
				To:   time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).UnixMilli()},
		}

		log.WithField("tenant", tenant).Info("Start Participant sync")
		if err = mqttclient.SendEbmsMessage(ebmsMessage); err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondWithStatus(w, http.StatusNoContent)
	}
}

func syncMeterpointEda() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		var m model.MeteringPoint
		err := json.NewDecoder(r.Body).Decode(&m)
		if err != nil {
			log.Errorf("Body Parsing. %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		eeg, err := database.GetEeg(tenant)
		if err != nil {
			log.WithField("error", err).Error("Query EEG")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		day := time.Now()
		ebmsMessage := model.EbmsMessage{
			Sender:      strings.ToUpper(tenant),
			Receiver:    strings.ToUpper(eeg.GridOperator),
			MessageCode: model.EBMS_ZP_SYNC,
			Meter:       &model.Meter{MeteringPoint: m.MeteringPoint},
			Timeline: &model.Timeline{
				From: time.Date(day.Year(), day.Month(), day.Day()-3, 0, 0, 0, 0, day.Location()).UnixMilli(),
				To:   time.Date(day.Year(), day.Month(), day.Day()-2, 0, 0, 0, 0, day.Location()).UnixMilli()},
		}

		log.WithField("tenant", tenant).Info("Start Metering sync")
		if err = mqttclient.SendEbmsMessage(ebmsMessage); err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondWithStatus(w, http.StatusNoContent)
	}
}

func uploadMasterData() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		// Parse our multipart form, 10 << 20 specifies a maximum
		// upload of 10 MB files.
		var err error = r.ParseMultipartForm(10 << 20)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		sheet := r.FormValue("sheet")

		file, handler, err := r.FormFile("masterdatafile")
		if err != nil {
			glog.Error(err)
			respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}
		defer file.Close()
		glog.Infof("--- Upload File: %s, %s, %s\n", sheet, handler.Filename, tenant)

		if err = database.ImportMasterdataFromExcel(file, handler.Filename, sheet, tenant); err != nil {
			glog.Error(err)
			respondWithError(w, http.StatusBadRequest, err.Error())
		} else {
			glog.Infof("Import File %s successful", handler.Filename)
			w.WriteHeader(http.StatusOK)
		}
	}
}
