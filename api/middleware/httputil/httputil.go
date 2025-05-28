package httputil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ErrUnanticipatedResponse struct {
	Status      int
	ContentType string
	Body        string
}

func NewErrUnanticipatedResponse(resp *http.Response) *ErrUnanticipatedResponse {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return &ErrUnanticipatedResponse{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        string(body),
	}
}

func (err ErrUnanticipatedResponse) Error() string {
	return fmt.Sprintf(
		"unanticipated response %d: (%s) %s",
		err.Status, err.ContentType, err.Body,
	)
}

func Ensure2XX(resp *http.Response) error {
	if resp.StatusCode >= 300 {
		return NewErrUnanticipatedResponse(resp)
	}
	return nil
}

func DecodeJSONResponse(resp *http.Response, obj interface{}) error {
	if !strings.Contains(resp.Header.Get("Content-Type"), "application/json") || resp.StatusCode >= 300 {
		return NewErrUnanticipatedResponse(resp)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, obj)
}
