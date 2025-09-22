package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"net/http"
)

func InitApiRouter(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/master").Subrouter()

	s.HandleFunc("/updatepartfact", middleware.ProtectApi(updateParticipantFactorAPI())).Methods("POST")
	s.HandleFunc("/test", testApi).Methods("GET")
	return r
}

func testApi(w http.ResponseWriter, r *http.Request) {

	println("TestApi")

	return
}

func updateParticipantFactorAPI() middleware.JWTHandlerFunc {
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

		if request.MeteringPoints == nil || len(request.MeteringPoints) == 0 {
			log.WithField("tenant", tenant).WithError(err).Error("failed to request metering point PRTFACT")
			respondWithError(w, http.StatusBadRequest, "failed to request metering point PRTFACT")
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
		respondWithStatus(w, http.StatusCreated)
	}
}
