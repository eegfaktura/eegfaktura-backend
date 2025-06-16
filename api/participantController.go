package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/util"
	"encoding/json"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"net/http"
)

func InitParticipantRouter(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/participant").Subrouter()

	s.HandleFunc("", middleware.Protect(fetchParticipant())).Methods("GET")
	s.HandleFunc("", middleware.Protect(registerParticipant())).Methods("POST")
	s.HandleFunc("/{id}", middleware.Protect(updateParticipant())).Methods("PUT")
	s.HandleFunc("/{id}", middleware.Protect(archiveParticipant())).Methods("DELETE")
	// Commit a participant to be a member of a EEG
	s.HandleFunc("/{id}/confirm", middleware.Protect(confirmParticipant())).Methods("POST")
	s.HandleFunc("/v2/{id}", middleware.Protect(updateParticipantPartial())).Methods("PUT")

	return r
}

func fetchParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		db, err := database.ConnectToDatabase()
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to fetch participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		participant, err := database.GetParticipants(db, tenant)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to fetch participant.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		//time.Sleep(2 * time.Second)
		respondWithJSON(w, 200, participant)
	}
}

func updateParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		//vars := mux.Vars(r)
		//participantId := vars["id"]

		var t model.EegParticipant
		err := json.NewDecoder(r.Body).Decode(&t)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.ConnectToDatabase()
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		err = database.UpdateParticipant(db, tenant, claims.Username, &t)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update participant.")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, http.StatusAccepted, t)
	}
}

func updateParticipantPartial() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["id"]

		var p map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&p)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update partial participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}
		db, err := database.ConnectToDatabase()
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update partial participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		name := p["path"].(string)
		value := p["value"]

		err = database.UpdateParticipantPartial(db, participantId, name, value)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update partial participant.")
			respondWith(w, http.StatusInternalServerError, tenant, err)
			return
		}

		participant, err := database.QueryParticipant(db, participantId)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to update partial participant.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, participant)
	}
}

func registerParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		var t model.EegParticipant
		err := json.NewDecoder(r.Body).Decode(&t)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.ConnectToDatabase()
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		tx, err := db.Beginx()
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register participant.")
			respondWith(w, http.StatusInternalServerError, tenant, model.ErrOpenTx(err))
			return
		}
		defer func() {
			if err != nil {
				_ = tx.Rollback()
			} else {
				_ = tx.Commit()
			}
		}()

		err = database.RegisterParticipant(tx, tenant, claims.Username, &t)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to register participant.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusCreated, t)
	}
}

func confirmParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {

		vars := mux.Vars(r)
		participantId := vars["id"]

		var meters []*model.MeteringPoint

		err := json.NewDecoder(r.Body).Decode(&meters)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to confirm participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.ConnectToDatabase()
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to confirm participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		eeg, err := database.GetEeg(db, tenant)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to confirm participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
			return
		}
		participant, err := database.QueryParticipant(db, participantId)
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to confirm participant.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}

		if err = database.ConfirmParticipant(db, claims.Username, participantId); err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to confirm participant.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		participant.Status = model.ACTIVE

		participantMetersMap := make(map[string]*model.MeteringPoint)
		for _, meter := range participant.MeteringPoint {
			participantMetersMap[meter.MeteringPoint] = meter
		}

		if eeg.Online {
			for _, qm := range meters {
				if m, ok := participantMetersMap[qm.MeteringPoint]; ok {
					m.ActivationMode = qm.ActivationMode
					m.ActivationCode = qm.ActivationCode
					var from int64
					if qm.RegisteredSince.After(participant.ParticipantSince.Date) {
						from = util.CalcProcessDate(qm.RegisteredSince)
					} else {
						from = util.CalcProcessNullDate(participant.ParticipantSince)
					}

					log.WithField("tenant", tenant).Infof("Start Meteringpoint %s registration - Active at: %+v", m.MeteringPoint, from)
					if m.ActivationMode == model.ONLINE {
						if err = mqttclient.RegistrationForParticipation(eeg, m, &from); err != nil {
							log.WithField("tenant", tenant).WithError(err).Error("failed to confirm participant.")
							respondWith(w, http.StatusBadRequest, tenant, err)
							return
						}
					} else {
						if err = mqttclient.OfflineRegistrationForParticipation(eeg, m, &from); err != nil {
							log.WithField("tenant", tenant).WithError(err).Error("failed to confirm participant.")
							respondWith(w, http.StatusBadRequest, tenant, err)
							return
						}
					}
					//err = database.MeteringPointsSetStatus(db, tenant, model.INIT, 0, []string{m.MeteringPoint}, nil, nil)
					//if err != nil {
					//	respondWith(w, http.StatusBadRequest, tenant, err)
					//	return
					//}
				}
			}
		} else {
			meterIds := []string{}
			for _, m := range participant.MeteringPoint {
				meterIds = append(meterIds, m.MeteringPoint)
				m.Status = model.S_ACTIVE
			}
			err := database.MeteringPointsSetStatus(db, tenant, model.ACTIVE, nil, meterIds, nil, nil)
			if err != nil {
				log.WithField("tenant", tenant).WithError(err).Error("failed to confirm participant.")
				respondWith(w, http.StatusBadRequest, tenant, err)
				return
			}
		}
		respondWithJSON(w, http.StatusCreated, participant)
	}
}

func archiveParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		idStr := vars["id"]

		db, err := database.ConnectToDatabase()
		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to archive participant.")
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		if err := database.ArchiveParticipant(db, claims.Username, idStr); err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("failed to archive participant.")
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, map[string]interface{}{"status": "ok"})
	}
}
