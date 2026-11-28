package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"github.com/gorilla/mux"
	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#272: „Aktiv ab“ as the requested date. The tests fix today to 26.11.2026: the window is 27.11.2026 to
// 13.01.2027.

var adToday = civil.DateFor(2026, 11, 26)

func fixToday(t *testing.T, d civil.Date) {
	t.Helper()
	orig := today
	t.Cleanup(func() { today = orig })
	today = func() civil.Date { return d }
}

// captureRegistrations replaces the outgoing registration (ANFORDERUNG_ECON) and records the start date per meter.
func captureRegistrations(t *testing.T) map[string]civil.Date {
	t.Helper()
	orig := mqttclient.RegistrationForParticipation
	t.Cleanup(func() { mqttclient.RegistrationForParticipation = orig })
	sent := map[string]civil.Date{}
	mqttclient.RegistrationForParticipation = func(eeg *model.Eeg, meter *model.MeteringPoint, from *int64) error {
		sent[meter.MeteringPoint] = civil.DateOf(time.UnixMilli(*from).UTC())
		return nil
	}
	return sent
}

type adFixture struct {
	t           *testing.T
	tenant      string
	participant string
}

func (f adFixture) exec(q string, args ...interface{}) {
	f.t.Helper()
	_, err := testDB.DbInstance.Exec(q, args...)
	require.NoError(f.t, err)
}

// newAdFixture creates a community (online or not) with one member; status and "member since" are
// those of the member. Everything is removed after the test.
func newAdFixture(t *testing.T, tenant string, online bool, status string, participantSince interface{}) adFixture {
	f := adFixture{t: t, tenant: tenant, participant: "c2720000-0000-4000-8000-00" + tenant[2:] + "0000"}
	cleanup := func() {
		f.exec(`DELETE FROM base.participant WHERE tenant = $1`, tenant)
		f.exec(`DELETE FROM base.eeg WHERE tenant = $1`, tenant)
	}
	cleanup()
	t.Cleanup(cleanup)
	f.exec(`INSERT INTO base.eeg (tenant, name, description, "rcNumber", area, gridoperator_code, gridoperator_name,
		"communityId", street, "streetNumber", city, zip, email, online)
		VALUES ($1, 'Aktiv ab', 'Aktiv ab', $2, 'LOCAL', 'AT003000', 'Netz OOE', 'AT00300000000TC000272000000000001',
		'Weg', '1', 'Linz', '4020', 'office@example.org', $3)`, tenant, tenant, online)
	f.exec(`INSERT INTO base.participant (id, tenant, firstname, lastname, status, "participantSince", "createdBy", "lastModifiedBy")
		VALUES ($1, $2, 'Aktiv', 'Ab', $3, $4, 'test', 'test')`, f.participant, tenant, status, participantSince)
	for _, typ := range []string{"BILLING", "RESIDENCE"} {
		f.exec(`INSERT INTO base.address (participant_id, type, street, "streetNumber", city, zip)
			VALUES ($1, $2, 'Weg', '1', 'Linz', '4020')`, f.participant, typ)
	}
	return f
}

// meter adds a metering point of the member; activeSince nil = none.
func (f adFixture) meter(meter, status, processState, registeredSince string, activeSince interface{}) {
	f.exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, direction, status, process_state,
		"modifiedAt", "modifiedBy", "registeredSince", activesince, grid_operator_id)
		VALUES ($1, $2, $3, 'CONSUMPTION', $4, $5, now(), 'test', $6, $7, 'AT003000')`,
		meter, f.participant, f.tenant, status, processState, registeredSince, activeSince)
	f.exec(`INSERT INTO base.metering_partition_factor (metering_point_id, participant_id, tenant, "partFact", "createdBy")
		VALUES ($1, $2, $3, 100, 'test')`, meter, f.participant, f.tenant)
}

// dates is registeredSince and activesince of a metering point ("" = none).
func (f adFixture) dates(meter string) (registered, active string) {
	f.t.Helper()
	require.NoError(f.t, testDB.DbInstance.QueryRow(`SELECT to_char("registeredSince", 'YYYY-MM-DD'),
		COALESCE(to_char(activesince, 'YYYY-MM-DD'), '') FROM base.meteringpoint WHERE metering_point_id = $1
		AND participant_id = $2`, meter, f.participant).Scan(&registered, &active))
	return
}

func call(h func(http.ResponseWriter, *http.Request, *middleware.PlatformClaims, string), method, body string,
	vars map[string]string, tenant string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, "/", strings.NewReader(body))
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()
	h(w, req, &middleware.PlatformClaims{Username: "aktiv-ab-test"}, tenant)
	return w
}

func TestRegistrationWindowEndpoint(t *testing.T) {
	fixToday(t, adToday)
	r := mux.NewRouter()
	InitMeteringRouter(r, nil)
	var match mux.RouteMatch
	req, _ := http.NewRequest("GET", "/meteringpoint/registration-window", nil)
	require.True(t, r.Match(req, &match), "route registered")

	w := call(NewMeteringHandler(nil).registrationWindow(), "GET", "", nil, "TE000272")

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"from":"2026-11-27","to":"2027-01-13"}`, w.Body.String())
}

func TestConfirmParticipant_startDateFromStoredMeter(t *testing.T) {
	fixToday(t, adToday)
	db, err := database.GetDB(context.Background())
	require.NoError(t, err)
	confirm := func(f adFixture, body string) *httptest.ResponseRecorder {
		return call(NewParticipantHandler(db).confirmParticipant(), "POST", body, map[string]string{"id": f.participant}, f.tenant)
	}
	participantStatus := func(f adFixture) string {
		var s string
		require.NoError(t, testDB.DbInstance.Get(&s, `SELECT status FROM base.participant WHERE id = $1`, f.participant))
		return s
	}

	t.Run("each meter with its stored date, not the one of the body", func(t *testing.T) {
		f := newAdFixture(t, "TE002721", true, "NEW", "2026-12-01")
		f.meter("AT0030000000000000000000000272101", "INIT", "NEW", "2027-01-01", nil)
		f.meter("AT0030000000000000000000000272102", "INIT", "NEW", "2026-11-01", nil)
		sent := captureRegistrations(t)

		w := confirm(f, `[{"meteringPoint":"AT0030000000000000000000000272101","registeredSince":"2026-12-24"},
			{"meteringPoint":"AT0030000000000000000000000272102","registeredSince":"2027-01-10"}]`)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, map[string]civil.Date{
			"AT0030000000000000000000000272101": civil.DateFor(2027, 1, 1),
			"AT0030000000000000000000000272102": civil.DateFor(2026, 12, 1), // "member since" is later
		}, sent)
	})

	t.Run("one meter beyond the window: nothing sent, member not confirmed", func(t *testing.T) {
		f := newAdFixture(t, "TE002722", true, "NEW", "2026-01-01")
		f.meter("AT0030000000000000000000000272103", "INIT", "NEW", "2027-01-13", nil)
		f.meter("AT0030000000000000000000000272104", "INIT", "NEW", "2027-01-14", nil)
		sent := captureRegistrations(t)

		w := confirm(f, `[{"meteringPoint":"AT0030000000000000000000000272103"},{"meteringPoint":"AT0030000000000000000000000272104"}]`)

		assertErrorCode(t, w, 1208, "13.01.2027")
		assert.Empty(t, sent)
		assert.Equal(t, "NEW", participantStatus(f))
	})

	t.Run("member since beyond the window: nothing sent", func(t *testing.T) {
		f := newAdFixture(t, "TE002723", true, "NEW", "2027-03-01")
		f.meter("AT0030000000000000000000000272105", "INIT", "NEW", "2026-12-01", nil)
		sent := captureRegistrations(t)

		w := confirm(f, `[{"meteringPoint":"AT0030000000000000000000000272105"}]`)

		assertErrorCode(t, w, 1208, "01.03.2027")
		assert.Empty(t, sent)
		assert.Equal(t, "NEW", participantStatus(f))
	})
}

func TestCreateMeteringPoint_requestedDate(t *testing.T) {
	fixToday(t, adToday)
	h := newTestMeteringHandler(t)
	create := func(f adFixture, meter, extra string) *httptest.ResponseRecorder {
		body := `{"meteringPoint":"` + meter + `","direction":"CONSUMPTION","status":"INIT","processState":"NEW"` + extra + `}`
		return call(h.createMeteringPoint(), "PUT", body, map[string]string{"pid": f.participant}, f.tenant)
	}

	t.Run("online: the requested date is stored and sent", func(t *testing.T) {
		f := newAdFixture(t, "TE002724", true, "ACTIVE", "2026-01-01")
		sent := captureRegistrations(t)
		w := create(f, "AT0030000000000000000000000272201", `,"registeredSince":"2027-01-01"`)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		registered, active := f.dates("AT0030000000000000000000000272201")
		assert.Equal(t, "2027-01-01", registered)
		assert.Equal(t, "", active, "no provisional activation date")
		assert.Equal(t, map[string]civil.Date{"AT0030000000000000000000000272201": civil.DateFor(2027, 1, 1)}, sent)
	})

	t.Run("online: without a date today, sent for tomorrow", func(t *testing.T) {
		f := newAdFixture(t, "TE002724", true, "ACTIVE", "2026-01-01")
		sent := captureRegistrations(t)
		w := create(f, "AT0030000000000000000000000272202", "")

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		registered, _ := f.dates("AT0030000000000000000000000272202")
		assert.Equal(t, "2026-11-26", registered)
		assert.Equal(t, civil.DateFor(2026, 11, 27), sent["AT0030000000000000000000000272202"])
	})

	t.Run("online: beyond the window rejected, nothing saved or sent", func(t *testing.T) {
		f := newAdFixture(t, "TE002724", true, "ACTIVE", "2026-01-01")
		sent := captureRegistrations(t)
		w := create(f, "AT0030000000000000000000000272203", `,"registeredSince":"2027-01-14"`)

		assertErrorCode(t, w, 1208, "13.01.2027")
		assert.Empty(t, sent)
		var n int
		require.NoError(t, testDB.DbInstance.Get(&n, `SELECT count(*) FROM base.meteringpoint WHERE metering_point_id = $1`,
			"AT0030000000000000000000000272203"))
		assert.Equal(t, 0, n)
	})

	t.Run("offline: registeredSince is today as before", func(t *testing.T) {
		f := newAdFixture(t, "TE002725", false, "ACTIVE", "2026-01-01")
		w := create(f, "AT0030000000000000000000000272204", `,"registeredSince":"2027-05-01"`)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		registered, _ := f.dates("AT0030000000000000000000000272204")
		assert.Equal(t, civil.Today().String(), registered)
	})
}

func TestRegisterMeteringPoint_from(t *testing.T) {
	fixToday(t, adToday)
	h := newTestMeteringHandler(t)
	const meter = "AT0030000000000000000000000272301"
	register := func(f adFixture, from int64) *httptest.ResponseRecorder {
		body := `{"meteringPoint":"` + meter + `","direction":"CONSUMPTION","from":` + strconv.FormatInt(from, 10) + `}`
		return call(h.registerMeteringPoint(), "POST", body, map[string]string{"pid": f.participant}, f.tenant)
	}
	// web sends local midnight of the chosen day
	viennaMidnight := func(y int, m time.Month, d int) int64 {
		loc, err := time.LoadLocation("Europe/Vienna")
		require.NoError(t, err)
		return time.Date(y, m, d, 0, 0, 0, 0, loc).UnixMilli()
	}

	t.Run("from is stored and sent", func(t *testing.T) {
		f := newAdFixture(t, "TE002726", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "REJECTED", "2026-12-01", nil)
		sent := captureRegistrations(t)

		w := register(f, viennaMidnight(2027, time.January, 8))

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		registered, _ := f.dates(meter)
		assert.Equal(t, "2027-01-08", registered)
		assert.Equal(t, civil.DateFor(2027, 1, 8), sent[meter])
	})

	t.Run("from beyond the window: rejected, not stored, nothing sent", func(t *testing.T) {
		f := newAdFixture(t, "TE002726", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "REJECTED", "2026-12-01", nil)
		sent := captureRegistrations(t)

		w := register(f, viennaMidnight(2027, time.January, 14))

		assertErrorCode(t, w, 1208, "13.01.2027")
		registered, _ := f.dates(meter)
		assert.Equal(t, "2026-12-01", registered)
		assert.Empty(t, sent)
	})

	t.Run("without from the stored date applies", func(t *testing.T) {
		f := newAdFixture(t, "TE002726", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "REJECTED", "2026-12-01", nil)
		sent := captureRegistrations(t)

		w := register(f, 0)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, civil.DateFor(2026, 12, 1), sent[meter])
	})

	t.Run("start beyond the window because of member since: nothing stored or sent", func(t *testing.T) {
		f := newAdFixture(t, "TE002726", true, "ACTIVE", "2027-03-01")
		f.meter(meter, "INIT", "REJECTED", "2026-12-01", nil)
		sent := captureRegistrations(t)

		w := register(f, viennaMidnight(2027, time.January, 8))

		assertErrorCode(t, w, 1208, "01.03.2027")
		registered, _ := f.dates(meter)
		assert.Equal(t, "2026-12-01", registered)
		assert.Empty(t, sent)
	})
}
