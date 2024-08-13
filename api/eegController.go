package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strconv"
	"time"
)

func InitEegRouter(r *mux.Router) *mux.Router {
	s := r.PathPrefix("/eeg").Subrouter()

	s.HandleFunc("", middleware.Protect(getEEG())).Methods("GET")
	s.HandleFunc("", middleware.Protect(updateEEG())).Methods("POST")
	s.HandleFunc("/tariff", middleware.Protect(getTariff())).Methods("GET")
	s.HandleFunc("/tariff", middleware.Protect(addTariff())).Methods("POST")
	s.HandleFunc("/tariff/{id}", middleware.Protect(archiveTariff())).Methods("DELETE")
	s.HandleFunc("/sync/participants/{oid}", middleware.Protect(syncParticipantsEda())).Methods("POST")
	s.HandleFunc("/import/masterdata", middleware.Protect(uploadMasterData())).Methods("POST")
	s.HandleFunc("/export/masterdata", middleware.Protect(exportMasterData())).Methods("GET")
	s.HandleFunc("/notifications/{id}", middleware.Protect(notifications())).Methods("GET")
	s.HandleFunc("/gridoperators", middleware.Protect(gridOperators())).Methods("GET")

	return r
}

func getEEG() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		log.Infof("Query EEG with TENANT: %s", tenant)

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
		if eeg == nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1001, fmt.Sprintf("EEG %s is not existing yet!", tenant)))
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
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		if err = database.UpdateEegPartial(db, tenant, e); err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1002, err.Error()))
			return
		}
		eeg, err := database.GetEeg(db, tenant)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrGetEeg(err))
			return
		}
		respondWithJSON(w, 200, eeg)
	}
}

func getTariff() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		tariff, err := database.GetTariff(db, tenant)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
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
			respondWith(w, http.StatusBadRequest, tenant, model.ErrParseJson(err))
			return
		}
		log.Printf("ADD TARIF: %+v Tenant: %+v", t, tenant)
		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		if err = database.AddTariff(db, tenant, claims.Username, &t); err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusCreated, t)
	}
}

func archiveTariff() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		idStr := vars["id"]

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		if err := database.ArchiveTariff(db, tenant, idStr); err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}
		respondWithJSON(w, http.StatusAccepted, map[string]interface{}{"status": "ok"})
	}
}

func syncParticipantsEda() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		operatorId := vars["oid"]

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

		day := time.Now()
		from := time.Date(day.Year(), day.Month(), day.Day()-1, 0, 0, 0, 0, day.Location()).UnixMilli()
		to := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).UnixMilli()

		if err = mqttclient.RequestingMeteringPointList(eeg, operatorId, from, to); err != nil {
			respondWith(w, http.StatusInternalServerError, tenant, err)
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
			log.WithField("tanant", tenant).Error(err)
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1051, err.Error()))
			return
		}
		defer file.Close()
		log.Infof("--- Upload File: %s, %s, %s\n", sheet, handler.Filename, tenant)

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		if err = database.ImportMasterdataFromExcel(db, file, handler.Filename, sheet, tenant); err != nil {
			log.WithField("tanant", tenant).Error(err)
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1052, err.Error()))
		} else {
			log.Infof("Import File %s successful", handler.Filename)
			w.WriteHeader(http.StatusOK)
		}
	}
}

func exportMasterData() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
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

		participants, err := database.GetParticipants(db, tenant)
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, err)
			return
		}

		tariffMap, err := database.GetTariffNameMap(db, tenant)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1059, err.Error()))
			return
		}

		b, err := database.ExportMasterdataToExcel(participants, eeg, tariffMap)

		if err != nil {
			log.Errorf("Create Energy Export: %v", err.Error())
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

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		notifications, err := database.GetNotification(db, tenant, id, isAdmin())
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1055, err.Error()))
			return
		}
		respondWithJSON(w, 200, notifications)
	}
}

func gridOperators() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {

		db, err := database.ConnectToDatabase()
		if err != nil {
			respondWith(w, http.StatusBadRequest, tenant, model.ErrConnectDatabase(err))
			return
		}
		defer func() { _ = db.Close() }()

		o, err := database.GetGridOperators(db)
		if err != nil {
			respondWithHttpError(w, http.StatusBadRequest, BadProcessError(1055, err.Error()))
			return
		}
		respondWithJSON(w, 200, o)
	}
}
