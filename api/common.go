package api

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type HttpError struct {
	Code    int    `json:"code"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

func UnauthorizedError() HttpError {
	return HttpError{
		401,
		"Unauthorized",
		"You are not authorized to access this resource",
	}
}
func NotFoundError() *HttpError {
	return &HttpError{
		404,
		"Not found",
		"The requested resource was not found",
	}
}
func DataAccessLayerError(message string) *HttpError {
	return &HttpError{
		400,
		"Data access error",
		message,
	}
}
func BadRequestError(message string) *HttpError {
	return &HttpError{
		400,
		"Bad Request",
		message,
	}
}

func BadProcessError(code int, message string) *HttpError {
	return &HttpError{
		code,
		"Bad Process",
		message,
	}
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func respondWithError(w http.ResponseWriter, httpCode int, message string) {
	respondWithJSON(w, httpCode, map[string]string{"error": message})
}

func respondWithStatus(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
}

func respondWithHttpError(w http.ResponseWriter, httpCode int, error *HttpError) {
	respondWithJSON(w, httpCode, map[string]interface{}{"error": error})
}

func respondWith(w http.ResponseWriter, httpCode int, tenant string, data interface{}) {
	switch e := data.(type) {
	case *model.VfeegError:
		log.WithField("tenant", tenant).Error(e.Error())
		respondWithHttpError(w, httpCode, &HttpError{Error: e.Error(), Code: e.Code, Message: e.Error()})
		return
	default:
		respondWithJSON(w, httpCode, data)
	}
}
