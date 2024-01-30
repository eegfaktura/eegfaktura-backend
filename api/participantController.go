package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"encoding/json"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"net/http"
)

func InitParticipantRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/participant").Subrouter()

	s.HandleFunc("", middleware.Protect(fetchParticipant())).Methods("GET")
	s.HandleFunc("", middleware.Protect(registerParticipant())).Methods("POST")
	s.HandleFunc("/{id}", middleware.Protect(updateParticipant())).Methods("PUT")
	s.HandleFunc("/{id}", middleware.Protect(archiveParticipant())).Methods("DELETE")
	s.HandleFunc("/{id}/confirm", middleware.Protect(confirmParticipant())).Methods("POST")
	s.HandleFunc("/v2/{id}", middleware.Protect(updateParticipantPartial())).Methods("PUT")

	return r
}

func fetchParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		participant, err := database.GetParticipants(database.GetDBXConnection, tenant)
		if err != nil {
			log.WithField("tenant", tenant).WithField("error", "SQLQuery").Error(err.Error())
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1200, err.Error()))
			return
		}
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
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1299, err.Error()))
			return
		}

		err = database.UpdateParticipant(tenant, claims.Username, &t)
		if err != nil {
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		name := p["path"].(string)
		value := p["value"]

		_, err = database.UpdateParticipantPartial(database.GetDBXConnection, participantId, name, value)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		participant, err := database.GetParticipant(database.GetDBXConnection, participantId)
		//names := strings.Split(name, ".")
		//ret := map[string]interface{}{}
		//rr := ret
		//for _, n := range names[:len(names)-1] {
		//	rr[n] = make(map[string]interface{})
		//	rr = rr[n].(map[string]interface{})
		//}
		//rr[names[len(names)-1]] = value
		//fmt.Printf("ret: %+v\n", ret)
		respondWithJSON(w, http.StatusAccepted, participant)
	}
}

func registerParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		var t model.EegParticipant
		err := json.NewDecoder(r.Body).Decode(&t)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		db, err := database.GetDBXConnection()
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer db.Close()

		tx, err := db.Beginx()
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer tx.Rollback()

		err = database.RegisterParticipant(tx, tenant, claims.Username, &t)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tx.Commit()
		respondWithJSON(w, http.StatusCreated, t)
	}
}

func confirmParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {

		vars := mux.Vars(r)
		participantId := vars["id"]

		eeg, err := database.GetEeg(database.GetDBXConnection, tenant)
		if err != nil {
			log.WithField("tenant", tenant).Error(err)
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(500, err.Error()))
			return
		}
		participant, err := database.QueryParticipant(participantId)
		if err != nil {
			log.WithField("tenant", tenant).WithField("SQL", "QUERY Participant").Error(err)
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(500, err.Error()))
			return
		}

		if err = database.ConfirmParticipant(database.GetDBXConnection, claims.Username, participantId); err != nil {
			log.WithField("tenant", tenant).Error(err)
			return
		}
		participant.Status = model.ACTIVE

		if eeg.Online {
			for _, m := range participant.MeteringPoint {
				log.WithField("tenant", tenant).Infof("Start Meteringpoint %s registration", m.MeteringPoint)
				if err = mqttclient.RegistrationForParticipation(tenant, eeg, m); err != nil {
					respondWithError(w, http.StatusInternalServerError, err.Error())
					return
				}
			}
		} else {
			meterIds := []string{}
			for _, m := range participant.MeteringPoint {
				meterIds = append(meterIds, m.MeteringPoint)
				m.Status = model.ACTIVE
			}
			err := database.MeteringPointsSetStatus(database.GetDBXConnection, tenant, model.ACTIVE, meterIds)
			if err != nil {
				log.Errorf("Error SET PARTICIPANT ACTIVE: %+v", err.Error())
				http.Error(w, err.Error(), http.StatusBadRequest)
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

		if err := database.ArchiveParticipant(database.GetDBXConnection, claims.Username, idStr); err != nil {
			respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{"id": 500, "error": err.Error()})
			return
		}
		respondWithJSON(w, http.StatusAccepted, map[string]interface{}{"status": "ok"})
	}
}
