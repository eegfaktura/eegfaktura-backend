package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
)

func TestRequestMeteringPointValues(t *testing.T) {
	type args struct {
		tenant      string
		request     string
		mqttReqFunc func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error
	}

	tests := []struct {
		name  string
		args  args
		check func(t *testing.T, recorder *httptest.ResponseRecorder)
	}{
		{
			name: "Update EEG",
			args: args{
				tenant:  "TE001006",
				request: `{"meteringPoints": [{"meter": "AT000000000000000000001", "direction": "CONSUMPTION"}], "from": 1212001200120012, "to": 23423434243234234}`,
				mqttReqFunc: func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {
					return nil
				},
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, recorder.Code)
			},
		},
		{
			name: "Update EEG - sepp", // TODO: Add test cases.
			args: args{
				tenant:  "TE001006",
				request: fmt.Sprintf(`{"meteringPoints": [{"meter": "AT000000000000000000001", "direction": "CONSUMPTION"}], "from": %d, "to": 23423434243234234}`, time.Date(2023, time.Month(11), 1, 0, 0, 0, 0, time.Local).UnixMilli()),
				mqttReqFunc: func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {
					fmt.Printf("FromDate %s\n", time.UnixMilli(fromDate).String())
					assert.Equal(t, civil.DateFor(2024, 1, 1).Unix()*1000, fromDate)
					return nil
				},
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				fmt.Printf("recorder: %v\n", recorder)
				assert.Equal(t, http.StatusNoContent, recorder.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/meteringpoint/syncenergy", strings.NewReader(tt.args.request))
			w := httptest.NewRecorder()
			mqttclient.RequestingEnergyData = tt.args.mqttReqFunc
			requestMeteringPointValues()(w, req, nil, tt.args.tenant)
			tt.check(t, w)
		})
	}
}

func TestRequestChangePartitionFactor(t *testing.T) {
	data := `{"meteringPoints":[{"meter":"AT0030000000000000000000000953016","direction":"GENERATION","activation":"2022-01-01","partFact":1}]}`
	req, _ := http.NewRequest("POST", "/meteringpoint/changepartitionfactor", strings.NewReader(data))
	w := httptest.NewRecorder()
	mqttclient.ChangePartitionFactor = func(eeg *model.Eeg, meter []*model.ChangePartitionFactorRequest) error {
		assert.Equal(t, "TE001006", eeg.Id)
		return nil
	}

	requestChangePartitionFactor()(w, req, nil, "TE001006")

	assert.Equal(t, http.StatusNoContent, w.Code)
}
