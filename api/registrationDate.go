package api

import (
	"context"
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

// registrationRunning: the grid operator works on the registration; its start date can no longer change.
func registrationRunning(p model.ProcessStatusType) bool {
	return p == model.INIT || p == model.PENDING || p == model.APPROVED
}

// meterSaveRules applies the save rules of platform#272 to the stored metering point of an online EEG; offline
// EEGs and an unknown metering point are left as before. registeredSince and activeSince are what the save would
// write, nil when it does not carry the field. An activeSince that must not be written (metering point not active)
// is cleared, so the update leaves the column alone. Only the REST saves call it: the admin path (gRPC) stays open.
func meterSaveRules(eeg *model.Eeg, stored *model.MeteringPoint, registeredSince *civil.Date, activeSince *civil.NullDate, day civil.Date) error {
	if !eeg.Online || stored == nil {
		return nil
	}
	if registeredSince != nil && !registeredSince.Equal(stored.RegisteredSince) {
		if registrationRunning(stored.ProcessState) {
			return model.ErrMeterDateLocked(fmt.Errorf("„Aktiv ab“ des Zählpunkts %s lässt sich nicht ändern, "+
				"solange die Anmeldung läuft.", stored.MeteringPoint))
		}
		if stored.Status != model.S_ACTIVE {
			if err := checkRequestedDate(stored.MeteringPoint, *registeredSince, day); err != nil {
				return err
			}
		}
	}
	if activeSince == nil {
		return nil
	}
	if stored.Status != model.S_ACTIVE {
		// only the grid operator (ABSCHLUSS) or the support sets it; web always sends the whole metering point
		*activeSince = civil.NullDate{}
		return nil
	}
	storedSince := civil.NullDate{}
	if stored.State != nil {
		storedSince = stored.State.ActiveSince
	}
	if activeSince.Valid && (!storedSince.Valid || !activeSince.Date.Equal(storedSince.Date)) {
		return model.ErrMeterDateLocked(fmt.Errorf("„Aktiv ab“ des aktiven Zählpunkts %s ändert nur der Support.",
			stored.MeteringPoint))
	}
	return nil
}

// checkMeterSave loads the community and the stored metering point (QueryParticipant asserts the tenant) and applies
// meterSaveRules.
func (h *MeteringHandler) checkMeterSave(ctx context.Context, tenant, participantId, meterId string, registeredSince *civil.Date, activeSince *civil.NullDate) error {
	eeg, err := h.db.GetEegById(ctx, tenant)
	if err != nil {
		return model.ErrGetEeg(err)
	}
	if !eeg.Online {
		return nil
	}
	participant, err := h.db.QueryParticipant(ctx, tenant, participantId)
	if err != nil {
		return err
	}
	return meterSaveRules(eeg, meterOf(participant, meterId), registeredSince, activeSince, today())
}

// meterOf is the metering point meterId of the participant, nil when it has none.
func meterOf(participant *model.EegParticipant, meterId string) *model.MeteringPoint {
	for _, m := range participant.MeteringPoint {
		if m.MeteringPoint == meterId {
			return m
		}
	}
	return nil
}
