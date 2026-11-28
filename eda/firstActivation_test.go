package eda

import (
	"context"
	"fmt"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/model"
	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#272: the answer of the grid operator (ABSCHLUSS, and ABLEHNUNG with code 156 "already assigned") sets
// its date on the first activation of an entry, replacing an activesince entered by hand before.
func TestProtocolEcReqOnlHandler_firstActivationReplacesProvisionalDate(t *testing.T) {
	const (
		tenant      = "TE000002"
		ecId        = "AT00300000000TC000002000000000001"
		participant = "c2720000-0000-4000-8000-0000000e0272"
		completed   = "AT0030000000000000000000000272601"
		assigned    = "AT0030000000000000000000000272602"
	)
	exec := func(q string, args ...interface{}) {
		t.Helper()
		_, err := testDB.DbInstance.Exec(q, args...)
		require.NoError(t, err)
	}
	cleanup := func() {
		// other tests count the notifications and the history of TE000002
		exec(`DELETE FROM base.notification WHERE tenant = $1 AND notification::text LIKE '%' || $2 || '%'`, tenant, "272601")
		exec(`DELETE FROM base.notification WHERE tenant = $1 AND notification::text LIKE '%' || $2 || '%'`, tenant, "272602")
		exec(`DELETE FROM base.processhistory WHERE tenant = $1 AND "conversationId" LIKE '%27260_'`, tenant)
		exec(`DELETE FROM base.participant WHERE id = $1`, participant)
	}
	cleanup()
	t.Cleanup(cleanup)
	exec(`INSERT INTO base.participant (id, tenant, firstname, lastname, status, "createdBy", "lastModifiedBy")
		VALUES ($1, $2, 'First', 'Activation', 'ACTIVE', 'test', 'test')`, participant, tenant)
	for _, meter := range []string{completed, assigned} {
		exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, direction, status, process_state,
			"modifiedAt", "modifiedBy", "registeredSince", activesince, grid_operator_id)
			VALUES ($1, $2, $3, 'CONSUMPTION', 'INIT', 'APPROVED', now(), 'test', '2026-12-01', '2026-11-01', 'AT003000')`,
			meter, participant, tenant)
		exec(`INSERT INTO base.metering_partition_factor (metering_point_id, participant_id, tenant, "partFact", "createdBy")
			VALUES ($1, $2, $3, 100, 'test')`, meter, participant, tenant)
	}
	activeSince := func(meter string) (string, string) {
		var d, status string
		require.NoError(t, testDB.DbInstance.QueryRow(`SELECT to_char(activesince, 'YYYY-MM-DD'), status
			FROM base.meteringpoint WHERE metering_point_id = $1 AND participant_id = $2`, meter, participant).Scan(&d, &status))
		return d, status
	}
	ctx := context.Background()

	t.Run("ABSCHLUSS", func(t *testing.T) {
		// noon UTC: the same calendar day in every time zone of the runners
		activation := time.Date(2026, 12, 15, 12, 0, 0, 0, time.UTC).UnixMilli()
		msg, err := prepareNotificationMessage(fmt.Sprintf(`{"conversationId":"TE000002202612150000000000000272601",
			"messageId":"AT003000202612150000000000000272601","sender":"AT003000","receiver":"TE000002",
			"messageCode":"ABSCHLUSS_ECON","messageCodeVersion":"02.00","ecId":%q,"meterList":[{"meteringPoint":%q,
			"direction":"CONSUMPTION","activation":%d,"partFact":100,"from":%d,"to":253402210800000,
			"consentId":"AT003000202612150000000000272601"}]}`, ecId, completed, activation, activation),
			model.EBMS_ONLINE_REG_COMPLETION, model.EC_REQ_ONL, tenant, ecId, []int16{}, nil, nil)
		require.NoError(t, err)

		protocolEcReqOnlHandler(ctx, msg)

		d, status := activeSince(completed)
		assert.Equal(t, "2026-12-15", d)
		assert.Equal(t, "ACTIVE", status)
	})

	t.Run("ABLEHNUNG code 156: the day of the answer", func(t *testing.T) {
		msg, err := prepareNotificationMessage(fmt.Sprintf(`{"conversationId":"TE000002202612150000000000000272602",
			"messageId":"AT003000202612150000000000000272602","sender":"AT003000","receiver":"TE000002",
			"messageCode":"ABLEHNUNG_ECON","messageCodeVersion":"01.11","ecId":%q,
			"responseData":[{"meteringPoint":%q,"responseCode":[156]}]}`, ecId, assigned),
			model.EBMS_ONLINE_REG_REJECTION, model.EC_REQ_ONL, tenant, ecId, nil, nil, nil)
		require.NoError(t, err)

		protocolEcReqOnlHandler(ctx, msg)

		d, status := activeSince(assigned)
		assert.Equal(t, civil.Today().String(), d)
		assert.Equal(t, "ACTIVE", status)
	})
}
