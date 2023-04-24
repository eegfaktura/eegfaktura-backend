package api

import (
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/parser"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func InitParticipantRouter(r *mux.Router, jwtWrapper middleware.JWTWrapperFunc) *mux.Router {
	s := r.PathPrefix("/participant").Subrouter()

	s.HandleFunc("", jwtWrapper(fetchParticipant())).Methods("GET")
	s.HandleFunc("", jwtWrapper(registerParticipant())).Methods("POST")
	s.HandleFunc("/{id}", jwtWrapper(updateParticipant())).Methods("PUT")
	s.HandleFunc("/{id}/confirm", jwtWrapper(confirmParticipant())).Methods("POST")

	return r
}

func fetchParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		participant, err := database.GetParticipant(tenant)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, 200, participant)
	}
}

func updateParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		vars := mux.Vars(r)
		participantId := vars["id"]

		var t map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&t)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = database.UpdateParticipant(tenant, participantId, t)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithStatus(w, http.StatusAccepted)
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

		err = database.RegisterParticipant(tenant, claims.Username, &t)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respondWithJSON(w, http.StatusCreated, t)
	}
}

func confirmParticipant() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {

		vars := mux.Vars(r)
		participantId := vars["id"]

		// Parse our multipart form, 10 << 20 specifies a maximum
		// upload of 10 MB files.
		var err error = r.ParseMultipartForm(10 << 20)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		formdata := r.MultipartForm // ok, no problem so far, read the Form data

		//get the *fileheaders
		files := formdata.File["docfiles"] // grab the filenames

		for i, _ := range files { // loop through the files one by one
			file, err := files[i].Open()
			defer file.Close()
			if err != nil {
				fmt.Fprintln(w, err)
				return
			}

			outputPath := filepath.Join(viper.GetString("file-content.basedir"), tenant)
			err = os.MkdirAll(outputPath, os.ModePerm)
			if err != nil {
				fmt.Fprintf(w, "Unable to create the file for writing. Check your write access privilege %s", err.Error())
				return
			}
			out, err := os.Create(filepath.Join(outputPath, files[i].Filename))

			defer out.Close()
			if err != nil {
				fmt.Fprintf(w, "Unable to create the file for writing. Check your write access privilege %s", err.Error())
				return
			}

			_, err = io.Copy(out, file) // file not files[i] !

			if err != nil {
				fmt.Fprintln(w, err)
				return
			}

			log.Debug("Files uploaded successfully : ")
			fmt.Fprintf(w, files[i].Filename+"\n")
		}
		if err = database.ConfirmParticipant(tenant, claims.Username, participantId); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

		if err = parser.SendMailFromTemplate(tenant, participantId,
			filepath.Join(viper.GetString("file-content.templates"), tenant, "template/AktivierungsEmail-template.html"),
			"Aktivierung im Serviceportal",
			"obermueller.peter@gmail.com"); err != nil {
			fmt.Fprintf(w, err.Error())
			return
		}

	}
}
