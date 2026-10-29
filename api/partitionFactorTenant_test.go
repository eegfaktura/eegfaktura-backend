package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"github.com/stretchr/testify/assert"
)

// 261005-ca4: a partition factor change is only sent for metering points of the
// caller's community. AT...30041725 belongs to TE000002 in the fixture.
func TestRequestChangePartitionFactorRefusesForeignMeter(t *testing.T) {
	orig := mqttclient.ChangePartitionFactor
	t.Cleanup(func() { mqttclient.ChangePartitionFactor = orig })
	calls := 0
	mqttclient.ChangePartitionFactor = func(eeg *model.Eeg, meter []*model.ChangePartitionFactorRequest) error {
		calls++
		return nil
	}

	for _, tc := range []struct{ name, data string }{
		{"meter of another community", `{"meteringPoints":[{"meter":"AT0030000000000000000000030041725","direction":"GENERATION","activation":"2022-01-01","partFact":1}]}`},
		{"own and foreign meter", `{"meteringPoints":[{"meter":"AT0030000000000000000000000953016","direction":"GENERATION","activation":"2022-01-01","partFact":1},{"meter":"AT0030000000000000000000030041725","direction":"GENERATION","activation":"2022-01-01","partFact":1}]}`},
		{"unknown meter", `{"meteringPoints":[{"meter":"AT0030000000000000000000099999999","direction":"GENERATION","activation":"2022-01-01","partFact":1}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/meteringpoint/changepartitionfactor", strings.NewReader(tc.data))
			w := httptest.NewRecorder()

			newTestMeteringHandler(t).requestChangePartitionFactor()(w, req, nil, "TE001006")

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
	assert.Equal(t, 0, calls, "no partition factor change may be sent via MQTT")
}
