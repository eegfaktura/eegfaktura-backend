package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/parser"
	"at.ourproject/vfeeg-backend/util"
	"encoding/json"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func InitMeteringRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/meteringpoint").Subrouter()

	s.HandleFunc("/{pid}/update/{mid}", jwtWrapper(updateMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/create", jwtWrapper(createMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/register", jwtWrapper(registerMeteringPoint())).Methods("POST")
	s.HandleFunc("/{pid}/syncenergy", jwtWrapper(requestMeteringPointValues())).Methods("POST")

	return r
}

func createMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]

		var m model.MeteringPoint
		err := json.NewDecoder(r.Body).Decode(&m)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = database.RegisterMeteringPoint(tenant, participantId, &m)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if m.Status == model.NEW {
			log.WithField("tenant", tenant).Infof("register Meter:  %v ", m)
			eeg, err := database.GetEeg(tenant)
			if err != nil {
				log.WithField("error", err).Error("Query EEG")
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			participant, err := database.QueryParticipant(participantId)
			if err != nil {
				log.WithField("error", err).Error("Query Participant")
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if eeg.Online {
				ebmsMessage := model.EbmsMessage{
					Sender:      strings.ToUpper(tenant),
					Receiver:    strings.ToUpper(eeg.GridOperator),
					MessageCode: model.EBMS_ONLINE_REG_INIT,
					EcId:        eeg.CommunityId,
					Meter:       &model.Meter{MeteringPoint: m.MeteringPoint, Direction: m.Direction},
				}

				log.WithField("tenant", tenant).Infof("Start Meteringpoint %s registration", m.MeteringPoint)
				if err = mqttclient.SendEbmsMessage(ebmsMessage); err != nil {
					respondWithError(w, http.StatusInternalServerError, err.Error())
					return
				}

				if err = parser.SendMailFromTemplate(tenant,
					filepath.Join(viper.GetString("file-content.templates"), tenant, "template/AktivierungsEmail-template.html"),
					"Aktivierung im Serviceportal", participant); err != nil {
					log.Errorf("Error Sending Mail: %+v", err.Error())
					http.Error(w, err.Error(), http.StatusBadRequest)
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
			log.WithField("error", err).Error("Decode UpdateMessage Json")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = database.UpdateMeteringPoint(tenant, participantId, meterId, &m)
		if err != nil {
			log.WithField("error", err).Error("Update Meteringpoint")
			http.Error(w, err.Error(), http.StatusBadRequest)
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

func registerMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]

		request := registerMeterRequestType{}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			log.WithField("error", err).Error("Decode Metering Request (Register) Json")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		eeg, err := database.GetEeg(tenant)
		if err != nil {
			log.WithField("error", err).Error("Query EEG")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		participant, err := database.QueryParticipant(participantId)
		if err != nil {
			log.WithField("error", err).Error("Query Participant")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check Meter available in Participant
		meterExistsInParticipant := false
		for _, p := range participant.MeteringPoint {
			if p.MeteringPoint == request.MeteringPoint {
				meterExistsInParticipant = true
				break
			}
		}

		log.WithField("tenant", tenant).Infof("register Meter:  %v ", request)

		if eeg.Online && meterExistsInParticipant {
			ebmsMessage := model.EbmsMessage{
				Sender:      strings.ToUpper(tenant),
				Receiver:    strings.ToUpper(eeg.GridOperator),
				MessageCode: model.EBMS_ONLINE_REG_INIT,
				EcId:        eeg.CommunityId,
				Meter:       &model.Meter{MeteringPoint: request.MeteringPoint, Direction: request.Direction},
			}

			log.WithField("tenant", tenant).Infof("Start Meteringpoint %s registration", request.MeteringPoint)
			if err = mqttclient.SendEbmsMessage(ebmsMessage); err != nil {
				respondWithError(w, http.StatusInternalServerError, err.Error())
				return
			}

			if err = parser.SendMailFromTemplate(tenant,
				filepath.Join(viper.GetString("file-content.templates"), tenant, "template/AktivierungsEmail-template.html"),
				"Aktivierung im Serviceportal", participant); err != nil {
				log.Errorf("Error Sending Mail: %+v", err.Error())
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		respondWithJSON(w, http.StatusCreated, participant)
	}
}

func requestMeteringPointValues() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]

		request := registerMeterRequestType{}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			log.WithField("error", err).Error("Decode Metering Request (Sync) Message Json")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		eeg, err := database.GetEeg(tenant)
		if err != nil {
			log.WithField("error", err).Error("Query EEG")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		participant, err := database.QueryParticipant(participantId)
		if err != nil {
			log.WithField("error", err).Error("Query Participant")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check Meter available in Participant
		meterExistsInParticipant := false
		for _, p := range participant.MeteringPoint {
			if p.MeteringPoint == request.MeteringPoint {
				meterExistsInParticipant = true
				break
			}
		}

		fromDate := util.TruncateToStartOfDay(time.UnixMilli(request.From)).UnixMilli()
		toDate := util.TruncateToEndOfDay(time.UnixMilli(request.To)).UnixMilli()

		log.WithField("tenant", tenant).Infof("request Metering values %v (%d - %d)", request, fromDate, toDate)
		if eeg.Online && meterExistsInParticipant {
			ebmsMessage := model.EbmsMessage{
				Sender: strings.ToUpper(tenant),
				//Sender: strings.ToUpper("SEPP.GAUG"),
				Receiver: strings.ToUpper(eeg.GridOperator),
				//Receiver:    strings.ToUpper("OBERMUELLER.PETER"),
				MessageCode: model.EBMS_ZP_SYNC,
				Meter:       &model.Meter{MeteringPoint: request.MeteringPoint},
				Timeline: &model.Timeline{
					From: fromDate,
					To:   toDate},
			}
			log.WithField("tenant", tenant).Infof("Start Meteringpoint (%s) value request", request.MeteringPoint)
			if err = mqttclient.SendEbmsMessage(ebmsMessage); err != nil {
				respondWithError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		respondWithJSON(w, http.StatusCreated, participant)
	}
}
