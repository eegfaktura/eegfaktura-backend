package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/util"
	"github.com/gorilla/mux"
	"github.com/jjeffery/civil"
	log "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
)

func InitMeteringRouter(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/meteringpoint").Subrouter()

	s.HandleFunc("/{pid}/update/{mid}", middleware.Protect(updateMeteringPoint())).Methods("PUT")
	s.HandleFunc("/v2/{pid}/update/{mid}", middleware.Protect(updateMeteringPointPartial())).Methods("PUT")
	s.HandleFunc("/v2/{pid}/updateid/{mid}", middleware.Protect(updateMeteringPointId())).Methods("PUT")
	s.HandleFunc("/{pid}/update/{mid}/partfact", middleware.Protect(updateMeteringPointPartFact())).Methods("PUT")
	s.HandleFunc("/{spid}/{dpid}/move/{mid}", middleware.Protect(moveMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/remove/{mid}", middleware.Protect(removeMeteringPoint())).Methods("DELETE")
	s.HandleFunc("/{pid}/archive/{mid}", middleware.Protect(archiveMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/create", middleware.Protect(createMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{pid}/register", middleware.Protect(registerMeteringPoint())).Methods("POST")
	s.HandleFunc("/{pid}/revokemeters", middleware.Protect(requestRevokeMeteringPoint())).Methods("POST")
	s.HandleFunc("/syncenergy", middleware.Protect(requestMeteringPointValues())).Methods("POST")
	s.HandleFunc("/changepartitionfactor", middleware.Protect(requestChangePartitionFactor())).Methods("POST")

	return r
}

func createMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]

		var m model.MeteringPoint
		err := json.NewDecoder(r.Body).Decode(&m)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		m.ModifiedAt = civil.Now()
		if m.Status != model.S_ACTIVE {
			m.RegisteredSince = civil.Today()
		}
		m.ModifiedBy = null.StringFrom(claims.Username)

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		err = db.RegisterMeteringPoint(tenant, claims.Username, participantId, &m)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register metering point.")
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1111, err.Error()))
			return
		}

		if m.ProcessState == model.NEW {
			log.WithField("tenant", tenant).Infof("register Meter:  %+v ", m)
			eeg, err := db.GetEegById(tenant)
			if err != nil {
				respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
				return
			}

			participant, err := db.QueryParticipant(participantId)
			if err != nil {
				log.WithField("tenant", tenant).WithError(err).Error("failed to register metering point. Cannot find appropriate participant.")
				respondWith(w, http.StatusBadRequest, tenant, err)
				return
			}

			if participant.Status == model.ACTIVE {
				var from int64
				if m.RegisteredSince.After(participant.ParticipantSince.Date) {
					from = util.CalcProcessDate(m.RegisteredSince)
				} else {
					from = util.CalcProcessNullDate(participant.ParticipantSince)
				}

				if err = edaRegisterMeteringpoint(eeg, m.ActivationMode, &m, &from); err != nil {
					respondWith(w, http.StatusBadRequest, tenant, err)
					return
				}
			}
			//if eeg.Online {
			//	if m.ActivationMode == model.ONLINE {
			//		if err = mqttclient.RegistrationForParticipation(eeg, &m); err != nil {
			//			respondWith(w, http.StatusBadRequest, tenant, err)
			//			return
			//		}
			//	} else if m.ActivationMode == model.OFFLINE {
			//		if err = mqttclient.OfflineRegistrationForParticipation(eeg, &m); err != nil {
			//			respondWith(w, http.StatusBadRequest, tenant, err)
			//			return
			//		}
			//	} else {
			//		log.WithField("tenant", tenant).Errorf("Wrong activation code! Meter: %+v", m)
			//		respondWith(w, http.StatusBadRequest, tenant, model.ErrWrongActivationCode(errors.New("Wrong activation code")))
			//		return
			//	}
			//}
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
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}
		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		m.ModifiedAt = civil.Now()
		m.ModifiedBy = null.StringFrom(claims.Username)
		err = db.UpdateMeteringPoint(tenant, claims.Username, participantId, meterId, &m)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, m)
	}

}

func updateMeteringPointPartial() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		pId := vars["pid"]
		mId := vars["mid"]

		var v map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&v)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update partial participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		name := v["path"].(string)
		value := v["value"]

		if err := db.UpdateMeteringPointPartial(tenant, claims.Username, pId, mId, map[string]interface{}{name: value}); err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrUpdateMeter(err))
			return
		}

		updatedMeter, err := db.FindAssignedMeteringById(tenant, mId)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrUpdateMeter(err))
			return
		}
		respondWithJSON(w, http.StatusAccepted, updatedMeter)
	}
}

func updateMeteringPointId() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		pId := vars["pid"]
		mId := vars["mid"]

		var v map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&v)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update partial participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		value := v["newId"].(string)

		if err := db.UpdateMeteringPointPartial(tenant, claims.Username, pId, mId, map[string]interface{}{"metering_point_id": value}); err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrUpdateMeter(err))
			return
		}

		updatedMeter, err := db.FindAssignedMeteringById(tenant, value)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrUpdateMeter(err))
			return
		}
		respondWithJSON(w, http.StatusAccepted, updatedMeter)
	}
}

func updateMeteringPointPartFact() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]
		meterId := vars["mid"]

		pf := struct {
			PartFact int `json:"partFact"`
		}{}
		err := json.NewDecoder(r.Body).Decode(&pf)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point partition factor.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}
		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point partition factor.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		ms, err := db.FindAllMeteringByTenant(tenant, participantId, []string{meterId})
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point partition factor.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}

		var meters []model.Meter
		for _, m := range ms {
			meters = append(meters, model.Meter{
				MeteringPoint: m.MeteringPoint,
				Direction:     m.Direction,
				PartFact:      pf.PartFact,
			})
		}

		if len(meters) == 1 {
			err = db.MeteringPointChangePartFactor(tenant, meters)
			if err != nil {
				log.WithField("tenant", tenant).WithError(err).Error("failed to update metering point partition factor.")
				respondWith(w, http.StatusBadRequest, tenant, err)
				return
			}
			ms[0].PartFact = pf.PartFact
			respondWithJSON(w, http.StatusAccepted, ms[0])
		} else {
			log.WithField("tenant", tenant).Errorf("failed to update metering point partition factor. Err: No PRTFACT specified %v", ms)
			respondWith(w, http.StatusBadRequest, tenant, &model.VfeegError{999, errors.New(fmt.Sprintf("No metering factor found N:%d", len(meters)))})
		}
	}
}

func moveMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		sParticipantId := vars["spid"]
		dParticipantId := vars["dpid"]
		meterId := vars["mid"]

		m := model.MeteringPoint{}
		err := json.NewDecoder(r.Body).Decode(&m)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to move metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}
		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to move metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		m.ModifiedAt = civil.Now()
		m.ModifiedBy = null.StringFrom(claims.Username)
		err = db.MoveMeteringPoint(tenant, claims.Username, sParticipantId, dParticipantId, meterId)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to move metering point.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, m)
	}
}

type registerMeterRequestType struct {
	MeteringPoint  string                 `json:"meteringPoint"`
	Direction      model.DirectionType    `json:"direction"`
	From           int64                  `json:"from"`
	To             int64                  `json:"to"`
	ActivationCode string                 `json:"activationCode,omitempty"`
	ActivationMode model.RegistrationMode `json:"activationMode,omitempty"`
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
			log.WithField("tenant", tenant).WithError(err).Error("failed to register metering point")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		eeg, err := db.GetEegById(tenant)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
			return
		}
		participant, err := db.QueryParticipant(participantId)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register metering point.")
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

		var from int64
		if meter.RegisteredSince.After(participant.ParticipantSince.Date) {
			from = util.CalcProcessDate(meter.RegisteredSince)
		} else {
			from = util.CalcProcessNullDate(participant.ParticipantSince)
		}

		meter.ActivationCode = request.ActivationCode
		if err = edaRegisterMeteringpoint(eeg, request.ActivationMode, meter, &from); err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}

		//if eeg.Online && meter != nil {
		//	if request.ActivationMode == model.ONLINE {
		//		if err = mqttclient.RegistrationForParticipation(eeg, meter); err != nil {
		//			respondWith(w, http.StatusBadRequest, tenant, err)
		//			return
		//		}
		//	} else {
		//		meter.ActivationCode = request.ActivationCode
		//		if err = mqttclient.OfflineRegistrationForParticipation(eeg, meter); err != nil {
		//			respondWith(w, http.StatusBadRequest, tenant, err)
		//			return
		//		}
		//	}
		//}
		log.WithField("tenant", tenant).Infof("register metering point. PID: %s, request: %+v", participantId, request)
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
			log.WithField("tenant", tenant).WithError(err).Error("failed to request metering point values.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to request metering point values.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		eeg, err := db.GetEegById(tenant)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to request metering point values.")
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
			log.WithField("tenant", tenant).Errorf("Request meter values - no Meter selected")
			respondWith(w, http.StatusInternalServerError, tenant, model.Wrap(errors.New("no Meter selected"), 3100))
			return
		}

		if eeg.Online {
			var errorList []string
			meters, err := db.FindMeteringByIds(tenant, meters)
			if err != nil {
				log.WithField("tenant", tenant).WithError(err).Error("failed to request metering point values.")
				respondWith(w, http.StatusInternalServerError, tenant, err)
				return
			}
			for _, m := range meters {
				if m.Status == model.S_INIT || m.State.Flag != model.F_ASSIGNED || !m.State.ActiveSince.Valid || !m.State.InactiveSince.Valid {
					continue
				}

				_activeSince := m.State.ActiveSince.Date.Unix() * 1000
				_inactiveSince := m.State.InactiveSince.Date.Unix() * 1000

				if _activeSince > toDate || _inactiveSince < fromDate {
					continue
				}

				from := util.MaxTimeStamp(_activeSince, fromDate)
				to := util.MinTimeStamp(_inactiveSince, toDate)

				if err = mqttclient.RequestingEnergyData(eeg, m, from, to); err != nil {
					log.WithField("tenant", tenant).Errorf("request Metering values %v (%s - %s)", m,
						time.UnixMilli(from).String(), time.UnixMilli(toDate).String())
					errorList = append(errorList, fmt.Sprintf("%s: %s", m.MeteringPoint, err.Error()))
				}
			}
			if errorList != nil && len(errorList) > 0 {
				log.WithField("tenant", tenant).Errorf("failed to request metering point values. Err: %v", errorList)
				respondWith(w, http.StatusInternalServerError, tenant, model.ErrRequestEnergyData(errors.New(strings.Join(errorList, "; "))))
				return
			}
		}
		respondWithStatus(w, http.StatusNoContent)
	}
}

func requestRevokeMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]
		log.WithField("tenant", tenant).Infof("Revoke meteringpoint in participant %s", participantId)

		request := struct {
			MeteringPoints []struct {
				Meter     string              `json:"meter"`
				Direction model.DirectionType `json:"direction"`
				ConsentId string              `json:"consentId"`
			} `json:"meteringPoints"`
			From   int64  `json:"from"`
			Reason string `json:"to"`
		}{}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to revoke metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to revoke metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		eeg, err := db.GetEegById(tenant)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to revoke metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
			return
		}
		participant, err := db.QueryParticipant(participantId)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to revoke metering point.")
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
			log.WithField("tenant", tenant).Error("failed to revoke metering point. Err: No Meter selected")
			respondWith(w, http.StatusInternalServerError, tenant, model.Wrap(errors.New("no Meter selected"), 3100))
			return
		}

		getReason := func(r *string) string {
			if r != nil {
				return *r
			}
			return ""
		}

		log.WithField("tenant", tenant).Infof("revoke Metering %v (%s - %s)", request, time.UnixMilli(fromDate).String(), getReason(reason))
		if eeg.Online {
			errorList := []string{}
			meters, err := db.FindActiveMeteringByIds(tenant, meters)
			if err != nil {
				respondWith(w, http.StatusInternalServerError, tenant, err)
				return
			}
			for _, m := range meters {
				if err = mqttclient.RevokeMeteringPoint(eeg, m, fromDate, reason); err != nil {
					log.WithField("tenant", tenant).Errorf("request revoke Metering %v (%d)", m, fromDate)
					errorList = append(errorList, fmt.Sprintf("%s: %s", m.MeteringPoint, err.Error()))
				}
			}
			if errorList != nil && len(errorList) > 0 {
				log.WithField("tenant", tenant).Errorf("failed to revoke metering point. Err: %v", errorList)
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

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to remove metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		err = db.RemoveMeteringPoint(tenant, participantId, meterId)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to remove metering point.")
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
		participantId := vars["pid"]

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to archive metering point.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		err = db.ArchiveMeteringPoint(tenant, participantId, meterId)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to archive metering point.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, map[string]interface{}{"meteringpoint": meterId})
	}
}

func requestChangePartitionFactor() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		var request struct {
			MeteringPoints []*model.ChangePartitionFactorRequest `json:"meteringPoints"`
		}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to request metering point PRTFACT")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.GetDB(context.Background())
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to request metering point PRTFACT")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}

		eeg, err := db.GetEegById(tenant)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
			return
		}

		if eeg.Online {
			if err = mqttclient.ChangePartitionFactor(eeg, request.MeteringPoints); err != nil {
				log.WithField("tenant", tenant).WithError(err).Errorf("failed to request metering point PRTFACT. Err: %v", request)
				respondWith(w, http.StatusInternalServerError, tenant, err)
			}
		} else {
			log.WithField("tenant", tenant).Warnf("Offline EEG want to change partitions of %+v", request)
			respondWithStatus(w, http.StatusNotFound)
			return
		}
		respondWithStatus(w, http.StatusNoContent)
	}
}

func edaRegisterMeteringpoint(eeg *model.Eeg, mode model.RegistrationMode, meter *model.MeteringPoint, from *int64) error {
	var err error
	if eeg.Online && meter != nil {
		if mode == model.ONLINE {
			if err = mqttclient.RegistrationForParticipation(eeg, meter, from); err != nil {
				return err
			}
		} else if mode == model.OFFLINE {
			if err = mqttclient.OfflineRegistrationForParticipation(eeg, meter, from); err != nil {
				return err
			}
		} else {
			return model.ErrWrongActivationCode(errors.New("Wrong activation code"))
		}
	}
	return nil
}
