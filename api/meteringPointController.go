package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/util"
	"encoding/json"
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

	s.HandleFunc("/{pid}/update/{mid}", jwtWrapper(updateMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/remove/{mid}", jwtWrapper(removeMeteringPoint())).Methods("DELETE")
	s.HandleFunc("/{pid}/archive/{mid}", jwtWrapper(archiveMeteringPoint())).Methods("PUT")
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
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1110, err.Error()))
			return
		}

		m.ModifiedAt = time.Now()
		if m.Status != model.ACTIVE {
			m.RegisteredSince = time.Now()
		}
		m.ModifiedBy = null.StringFrom(claims.Username)

		err = database.RegisterMeteringPoint(database.GetDBXConnection, tenant, claims.Username, participantId, &m)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1111, err.Error()))
			return
		}

		if m.Status == model.NEW {
			log.WithField("tenant", tenant).Infof("register Meter:  %v ", m)
			eeg, err := database.GetEeg(database.GetDBXConnection, tenant)
			if err != nil {
				log.WithField("error", "SQLQuery").Error(err.Error())
				respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1112, err.Error()))
				return
			}

			//participant, err := database.QueryParticipant(participantId)
			//if err != nil {
			//	log.WithField("error", err).Error("Query Participant")
			//	http.Error(w, err.Error(), http.StatusBadRequest)
			//	return
			//}

			if eeg.Online {
				if err = mqttclient.RegistrationForParticipation(tenant, eeg, &m); err != nil {
					respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1113, err.Error()))
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
			log.WithField("error", "DecodeJson").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1114, err.Error()))
			return
		}

		m.ModifiedAt = time.Now()
		m.ModifiedBy = null.StringFrom(claims.Username)
		err = database.UpdateMeteringPoint(database.GetDBXConnection, tenant, claims.Username, participantId, meterId, &m)
		if err != nil {
			log.WithField("error", "SQLUpdate").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1115, err.Error()))
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
			log.WithField("error", "DecodeJson").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1130, err.Error()))
			return
		}

		eeg, err := database.GetEeg(database.GetDBXConnection, tenant)
		if err != nil {
			log.WithField("error", "SQLQuery").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1131, err.Error()))
			return
		}
		participant, err := database.QueryParticipant(participantId)
		if err != nil {
			log.WithField("error", "SQLQuery").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1132, err.Error()))
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
				respondWithHttpError(w, http.StatusInternalServerError, BadProcessError(1140, err.Error()))
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
		log.WithField("tenant", tenant).Infof("Synchronize meteringpoint in participant %s", participantId)

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
			log.WithField("tenant", tenant).WithField("error", "DecodeJson").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1103, err.Error()))
			return
		}

		eeg, err := database.GetEeg(database.GetDBXConnection, tenant)
		if err != nil {
			log.WithField("tenant", tenant).WithField("error", "SQLQuery").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1000, err.Error()))
			return
		}
		participant, err := database.QueryParticipant(participantId)
		if err != nil {
			log.WithField("error", "Query").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1101, err.Error()))
			return
		}

		fromDate := util.TruncateToStartOfDay(time.UnixMilli(request.From)).UnixMilli()
		toDate := util.TruncateToEndOfDay(time.UnixMilli(request.To)).UnixMilli()

		log.WithField("tenant", tenant).Infof("request Metering values %v (%d - %d)", request, fromDate, toDate)
		if eeg.Online {
			var errorList []string
			for _, m := range request.MeteringPoints {
				if meter, err := database.FindMeteringById(database.GetDBXConnection, m.Meter); err == nil {
					if err = mqttclient.RequestingEnergyData(tenant, eeg, meter, fromDate, toDate); err != nil {
						log.WithField("tenant", tenant).Errorf("request Metering values %v (%d - %d)", m, fromDate, toDate)
						errorList = append(errorList, fmt.Sprintf("%s: %s", meter.MeteringPoint, err.Error()))
					}
				} else {
					log.WithField("tenant", tenant).Errorf("request Metering values %v (%d - %d)", m, fromDate, toDate)
					errorList = append(errorList, fmt.Sprintf("%s: %s", meter.MeteringPoint, err.Error()))
				}
			}
			if errorList != nil && len(errorList) > 0 {
				respondWithHttpError(w, http.StatusInternalServerError, BadProcessError(1100, strings.Join(errorList, "; ")))
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

		err := database.RemoveMeteringPoint(database.GetDBXConnection, tenant, participantId, meterId)
		if err != nil {
			log.WithField("tenant", tenant).WithField("error", "SQLDelete").Errorf("Remove Meteringpoint %s - %s", meterId, err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1155, err.Error()))
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

		err := database.MeteringPointsSetStatus(database.GetDBXConnection, tenant, model.ARCHIVED, []string{meterId})
		if err != nil {
			log.WithField("tenant", tenant).WithField("error", "SQLUpdate").Errorf("Remove Meteringpoint %s - %s", meterId, err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1156, err.Error()))
			return
		}
		respondWithJSON(w, http.StatusAccepted, map[string]interface{}{"meteringpoint": meterId})
	}
}
