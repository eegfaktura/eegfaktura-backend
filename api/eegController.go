package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strconv"
	"time"
)

func InitEegRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/eeg").Subrouter()

	s.HandleFunc("", middleware.Protect(getEEG())).Methods("GET")
	s.HandleFunc("", jwtWrapper(updateEEG())).Methods("POST")
	s.HandleFunc("/tariff", middleware.Protect(getTariff())).Methods("GET")
	s.HandleFunc("/tariff", jwtWrapper(addTariff())).Methods("POST")
	s.HandleFunc("/tariff/{id}", jwtWrapper(archiveTariff())).Methods("DELETE")
	s.HandleFunc("/sync/participants", jwtWrapper(syncParticipantsEda())).Methods("POST")
	s.HandleFunc("/import/masterdata", jwtWrapper(uploadMasterData())).Methods("POST")
	s.HandleFunc("/export/masterdata", jwtWrapper(exportMasterData())).Methods("GET")
	s.HandleFunc("/notifications/{id}", middleware.Protect(notifications())).Methods("GET")

	return r
}

func getEEG() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		log.Infof("Query EEG with TENANT: %s", tenant)
		eeg, err := database.GetEeg(tenant)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1000, err.Error()))
			return
		}
		respondWithJSON(w, 200, eeg)
	}
}

func updateEEG() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		var e map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&e)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1001, err.Error()))
			return
		}

		if err = database.UpdateEegPartial(tenant, e); err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1002, err.Error()))
			return
		}
		eeg, err := database.GetEeg(tenant)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1003, err.Error()))
			return
		}
		respondWithJSON(w, 200, eeg)
	}
}

func getTariff() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		tariff, err := database.GetTariff(tenant)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1010, err.Error()))
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
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1020, err.Error()))
			return
		}
		log.Printf("ADD TARIF: %+v Tenant: %+v", t, tenant)

		if err = database.AddTariff(database.GetDBXConnection, tenant, &t); err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1021, err.Error()))
			return
		}
		respondWithJSON(w, http.StatusCreated, t)
	}
}

func archiveTariff() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		idStr := vars["id"]

		if err := database.ArchiveTariff(database.GetDBXConnection, tenant, idStr); err != nil {
			if errors.Is(err, database.ErrTariffUtilized) {
				respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1022, err.Error()))
				return
			}
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1023, err.Error()))
			return
		}
		respondWithJSON(w, http.StatusAccepted, map[string]interface{}{"status": "ok"})
	}
}

func syncParticipantsEda() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		eeg, err := database.GetEeg(tenant)
		if err != nil {
			log.WithField("error", err).Error("Query EEG")
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1030, err.Error()))
			return
		}

		day := time.Now()
		from := time.Date(day.Year(), day.Month(), day.Day()-1, 0, 0, 0, 0, day.Location()).UnixMilli()
		to := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).UnixMilli()

		if err = mqttclient.RequestingMeteringPointList(tenant, eeg, from, to); err != nil {
			respondWithHttpError(w, http.StatusInternalServerError, BadProcessError(1031, err.Error()))
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
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1050, err.Error()))
			return
		}

		sheet := r.FormValue("sheet")

		file, handler, err := r.FormFile("masterdatafile")
		if err != nil {
			glog.Error(err)
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1051, err.Error()))
			return
		}
		defer file.Close()
		glog.Infof("--- Upload File: %s, %s, %s\n", sheet, handler.Filename, tenant)

		if err = database.ImportMasterdataFromExcel(database.GetDBXConnection, file, handler.Filename, sheet, tenant); err != nil {
			glog.Error(err)
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1052, err.Error()))
		} else {
			glog.Infof("Import File %s successful", handler.Filename)
			w.WriteHeader(http.StatusOK)
		}
	}
}

func exportMasterData() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		eeg, err := database.GetEeg(tenant)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1000, err.Error()))
			return
		}

		participants, err := database.GetParticipants(database.GetDBXConnection, tenant)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1050, err.Error()))
			return
		}

		tariffMap, err := database.GetTariffNameMap(tenant)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1059, err.Error()))
			return
		}

		b, err := database.ExportMasterdataToExcel(participants, eeg, tariffMap)

		if err != nil {
			glog.Errorf("Create Energy Export: %v", err.Error())
			respondWithHttpError(w, http.StatusInternalServerError, BadProcessError(1051, err.Error()))
			return
		}
		filename := fmt.Sprintf("%s-EEG-Masterdata-%s",
			tenant,
			time.Now().Format("20060102"),
		)

		w.Header().Set("Content-type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.xlsx"`, filename))
		w.Header().Set("filename", filename)

		if _, err := b.WriteTo(w); err != nil {
			fmt.Fprintf(w, "%s", err)
		}
	}
}

func notifications() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		idStr := vars["id"]

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1054, err.Error()))
			return
		}

		isAdmin := func() bool {
			for _, a := range claims.AccessGroups {
				if a == "/EEG_ADMIN" {
					return true
				}
			}
			return false
		}
		notifications, err := database.GetNotification(tenant, id, isAdmin())
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1055, err.Error()))
			return
		}
		respondWithJSON(w, 200, notifications)
	}
}
