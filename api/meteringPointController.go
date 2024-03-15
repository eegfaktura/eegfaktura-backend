package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/util"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
	"net/http"
	"strings"
	"time"
)

func InitMeteringRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/meteringpoint").Subrouter()

	s.HandleFunc("/{pid}/update/{mid}", middleware.Protect(updateMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/remove/{mid}", middleware.Protect(removeMeteringPoint())).Methods("DELETE")
	s.HandleFunc("/{pid}/archive/{mid}", middleware.Protect(archiveMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/create", middleware.Protect(createMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/register", middleware.Protect(registerMeteringPoint())).Methods("POST")
	s.HandleFunc("/{pid}/revokemeters", middleware.Protect(requestRevokeMeteringPoint())).Methods("POST")
	s.HandleFunc("/syncenergy", middleware.Protect(requestMeteringPointValues())).Methods("POST")

	return r
}

func createMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]

		var m model.MeteringPoint
		err := json.NewDecoder(r.Body).Decode(&m)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		m.ModifiedAt = time.Now()
		if m.Status != model.ACTIVE {
			m.RegisteredSince = time.Now()
		}
		m.ModifiedBy = null.StringFrom(claims.Username)

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		err = database.RegisterMeteringPoint(db, tenant, claims.Username, participantId, &m)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1111, err.Error()))
			return
		}

		if m.Status == model.NEW {
			log.WithField("tenant", tenant).Infof("register Meter:  %v ", m)
			eeg, err := database.GetEeg(db, tenant)
			if err != nil {
				respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
				return
			}

			if eeg.Online {
				if err = mqttclient.RegistrationForParticipation(tenant, eeg, &m); err != nil {
					respondWith(w, http.StatusBadRequest, tenant, err)
					return
				}
			}
		}
		respondWithJSON(w, http.StatusCreated, m)
	}
}

func updateMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]
		meterId := vars["mid"]

		m := model.MeteringPoint{}
		err := json.NewDecoder(r.Body).Decode(&m)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}
		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		m.ModifiedAt = time.Now()
		m.ModifiedBy = null.StringFrom(claims.Username)
		err = database.UpdateMeteringPoint(db, tenant, claims.Username, participantId, meterId, &m)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, m)
	}
}

type registerMeterRequestType struct {
	MeteringPoint string              `json:"meteringPoint"`
	Direction     model.DirectionType `json:"direction"`
	From          int64               `json:"from"`
	To            int64               `json:"to"`
}

// registerMeteringPoint activates existing meter at the net operator
//
// Here the registration only perform an online EDA communication
func registerMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]

		request := registerMeterRequestType{}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		eeg, err := database.GetEeg(db, tenant)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
			return
		}
		participant, err := database.QueryParticipant(db, participantId)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}

		// Check Meter available in Participant
		var meter *model.MeteringPoint
		for _, p := range participant.MeteringPoint {
			if p.MeteringPoint == request.MeteringPoint {
				meter = p
				break
			}
		}

		log.WithField("tenant", tenant).Infof("register Meter:  %v ", request)
		if eeg.Online && meter != nil {
			if err = mqttclient.RegistrationForParticipation(tenant, eeg, meter); err != nil {
				respondWith(w, http.StatusInternalServerError, tenant, err)
				return
			}
		}
		respondWithJSON(w, http.StatusCreated, participant)
	}
}

func requestMeteringPointValues() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {

		request := struct {
			MeteringPoints []struct {
				Meter     string              `json:"meter"`
				Direction model.DirectionType `json:"direction"`
			} `json:"meteringPoints"`
			From int64 `json:"from"`
			To   int64 `json:"to"`
		}{}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		eeg, err := database.GetEeg(db, tenant)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
			return
		}

		fromDate := util.TruncateToStartOfDay(time.UnixMilli(request.From)).UnixMilli()
		toDate := util.TruncateToEndOfDay(time.UnixMilli(request.To)).UnixMilli()

		var meters []string
		for _, m := range request.MeteringPoints {
			meters = append(meters, m.Meter)
		}

		if meters == nil {
			respondWith(w, http.StatusInternalServerError, tenant, model.Wrap(errors.New("no Meter selected"), 3100))
			return
		}

		log.WithField("tenant", tenant).Infof("request Metering values %v (%d - %d)", request, fromDate, toDate)
		if eeg.Online {
			var errorList []string
			meters, err := database.GetMeteringByIds(db, meters)
			if err != nil {
				respondWith(w, http.StatusInternalServerError, tenant, err)
				return
			}
			for _, m := range meters {
				if m.Status != model.ACTIVE || m.State.Active != model.P_ACTIVE {
					continue
				}
				if err = mqttclient.RequestingEnergyData(tenant, eeg, m, fromDate, toDate); err != nil {
					log.WithField("tenant", tenant).Errorf("request Metering values %v (%d - %d)", m, fromDate, toDate)
					errorList = append(errorList, fmt.Sprintf("%s: %s", m.MeteringPoint, err.Error()))
				}
			}
			if errorList != nil && len(errorList) > 0 {
				respondWith(w, http.StatusInternalServerError, tenant, model.ErrRequestEnergyData(errors.New(strings.Join(errorList, "; "))))
				return
			}
		}
		respondWithStatus(w, http.StatusCreated)
	}
}

func requestRevokeMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]
		log.WithField("tenant", tenant).Infof("Synchronize meteringpoint in participant %s", participantId)

		request := struct {
			MeteringPoints []struct {
				Meter     string              `json:"meter"`
				Direction model.DirectionType `json:"direction"`
			} `json:"meteringPoints"`
			From   int64  `json:"from"`
			Reason string `json:"to"`
		}{}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		eeg, err := database.GetEeg(db, tenant)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
			return
		}
		participant, err := database.QueryParticipant(db, participantId)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}

		fromDate := util.TruncateToStartOfDay(time.UnixMilli(request.From)).UnixMilli()
		var reason *string
		if len(strings.TrimSpace(request.Reason)) > 0 {
			reason = &request.Reason
		}

		var meters []string
		for _, m := range request.MeteringPoints {
			meters = append(meters, m.Meter)
		}

		if meters == nil {
			respondWith(w, http.StatusInternalServerError, tenant, model.Wrap(errors.New("no Meter selected"), 3100))
			return
		}

		log.WithField("tenant", tenant).Infof("revoke Metering %v (%d - %s)", request, fromDate, *reason)
		if eeg.Online {
			errorList := []string{}
			meters, err := database.GetMeteringByIds(db, meters)
			if err != nil {
				respondWith(w, http.StatusInternalServerError, tenant, err)
				return
			}
			for _, m := range meters {
				if err = mqttclient.RevokeMeteringPoint(tenant, eeg, m, fromDate, reason); err != nil {
					log.WithField("tenant", tenant).Errorf("request revoke Metering %v (%d)", m, fromDate)
					errorList = append(errorList, fmt.Sprintf("%s: %s", m.MeteringPoint, err.Error()))
				}
			}
			if errorList != nil && len(errorList) > 0 {
				respondWith(w, http.StatusInternalServerError, tenant, model.ErrRevokeMeter(errors.New(strings.Join(errorList, "; "))))
				return
			}
		}
		respondWithJSON(w, http.StatusCreated, participant)
	}
}

func removeMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]
		meterId := vars["mid"]

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		err = database.RemoveMeteringPoint(db, tenant, participantId, meterId)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, map[string]interface{}{"meteringpoint": meterId})
	}
}

func archiveMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		meterId := vars["mid"]
		//participantId := vars["pid"]

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		err = database.MeteringPointsSetStatus(db, tenant, model.ARCHIVED, []string{meterId})
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, map[string]interface{}{"meteringpoint": meterId})
	}
}
