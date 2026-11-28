package api

import (
	"fmt"
	"net/http"
	"time"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/util"
	"github.com/jjeffery/civil"
)

// „Aktiv ab“ as the requested date of a registration (platform#272, konzept-aktivierungsdatum.md): for a metering
// point of an online EEG that is not active, the requested date is stored in registeredSince. A registration starts
// at the later of it and "member since", at least tomorrow, and at most on the 30th working day after today.

// today is the calendar day in Austria; tests replace it.
var today = func() civil.Date { return util.DayInVienna(time.Now()) }

const germanDate = "02.01.2006"

type registrationWindowResponse struct {
	From civil.Date `json:"from"`
	To   civil.Date `json:"to"`
}

// registrationWindow answers the start dates a registration sent today may have, so that web and backend never
// count the holidays differently. No data of the community.
func (h *MeteringHandler) registrationWindow() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		from, to := util.RegistrationWindow(today())
		respondWithJSON(w, http.StatusOK, registrationWindowResponse{From: from, To: to})
	}
}

// registrationStart is the start date (ms) of the registration of meter, or ErrRegistrationDateBeyondWindow when it
// is after the window: then nothing may be sent.
func registrationStart(meter *model.MeteringPoint, participant *model.EegParticipant, day civil.Date) (int64, error) {
	start := util.RegistrationStart(meter.RegisteredSince, participant.ParticipantSince, day)
	if _, to := util.RegistrationWindow(day); start.After(to) {
		return 0, model.ErrRegistrationDateBeyondWindow(fmt.Errorf("Die Anmeldung des Zählpunkts %s würde am %s "+
			"beginnen, nach dem letzten erlaubten Tag %s. Eine Anmeldung darf höchstens %d Arbeitstage im Voraus "+
			"beginnen; es gilt der spätere Tag aus „Aktiv ab“ und „Mitglied seit“. Es wurde nichts gesendet.",
			meter.MeteringPoint, start.Format(germanDate), to.Format(germanDate), util.RegistrationWorkingDays))
	}
	return start.Unix() * 1000, nil
}

// checkRequestedDate rejects a requested date („Aktiv ab“) after the window before it is saved.
func checkRequestedDate(meterId string, requested, day civil.Date) error {
	if _, to := util.RegistrationWindow(day); requested.After(to) {
		return model.ErrRegistrationDateBeyondWindow(fmt.Errorf("„Aktiv ab“ %s des Zählpunkts %s liegt nach dem "+
			"letzten erlaubten Tag %s. Eine Anmeldung darf höchstens %d Arbeitstage im Voraus beginnen.",
			requested.Format(germanDate), meterId, to.Format(germanDate), util.RegistrationWorkingDays))
	}
	return nil
}
