package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"net/http"
)

func InitMeteringRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/meteringpoint").Subrouter()

	s.HandleFunc("/{pid}/update/{mid}", jwtWrapper(updateMeteringPoint())).Methods("PUT")
	s.HandleFunc("/{id}/create", jwtWrapper(registerMeteringPoint())).Methods("PUT")

	return r
}

func registerMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["id"]

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
		respondWithJSON(w, http.StatusCreated, m)
	}
}

func updateMeteringPoint() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["pid"]
		meterId := vars["mid"]

		var t map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&t)
		if err != nil {
			log.WithField("error", err).Error("Decode UpdateMessage Json")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = database.UpdateMeteringPoint(tenant, participantId, meterId, t)
		if err != nil {
			log.WithField("error", err).Error("Update Meteringpoint")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithStatus(w, http.StatusAccepted)
	}
}
