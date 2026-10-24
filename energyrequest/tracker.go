package energyrequest

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/pborman/uuid"
	"github.com/sirupsen/logrus"
)

// CheckInterval is how often open runs are evaluated.
const CheckInterval = 5 * time.Minute

// LookBack is how far back every evaluation (Check, lock, live entry) reads: the runs that can still be open, plus a
// margin for a check after a restart. All three read the rows from the same bound, so they attribute alike.
const LookBack = Deadline + time.Hour

// Store is the part of the database the summary needs (implemented by database.Database).
type Store interface {
	SaveEnergyRequestRow(ctx context.Context, tenant, rowType string, value interface{}) (int64, error)
	// EnergyRequestRows returns the run rows, answers and summaries of the last LookBack (ascending id), of the given
	// tenants or of all tenants (nil).
	EnergyRequestRows(ctx context.Context, tenants []string) ([]Row, error)
	TenantsByEcId(ctx context.Context, ecId string) ([]string, error)
	CloseEnergyRequest(ctx context.Context, tenant string, runRowId int64, summary Summary) (bool, error)
}

// checkMu keeps checks of this instance from overlapping.
var checkMu sync.Mutex

// NewRun starts collecting a run for a request of the community ecId for the period from..to.
func NewRun(ecId string, from, to int64) *Run {
	return &Run{Type: RowTypeRun, RunId: uuid.New(), EcId: ecId, From: from, To: to,
		Meters: []MeterRange{}, Skipped: []Skipped{}}
}

// Add records a metering point that is requested for from..to. A metering point that occurs twice (several
// participations in the period) is merged to the widest period.
func (r *Run) Add(meter string, from, to int64) {
	r.dropSkipped(meter)
	for i := range r.Meters {
		if r.Meters[i].MeteringPoint == meter {
			if from < r.Meters[i].From {
				r.Meters[i].From = from
			}
			if to > r.Meters[i].To {
				r.Meters[i].To = to
			}
			return
		}
	}
	r.Meters = append(r.Meters, MeterRange{MeteringPoint: meter, From: from, To: to})
}

// Skip records a metering point that is not requested, unless it is requested for another participation. Of
// several participations, the reason of the assigned one counts: NOT_ASSIGNED (an ended participation) gives way.
func (r *Run) Skip(meter, reason string) {
	r.SkipUntil(meter, reason, 0)
}

// SkipUntil is Skip with the time (ms) from which the metering point is free again (reason ALREADY_REQUESTED).
func (r *Run) SkipUntil(meter, reason string, until int64) {
	for _, m := range r.Meters {
		if m.MeteringPoint == meter {
			return
		}
	}
	for i, s := range r.Skipped {
		if s.MeteringPoint == meter {
			if s.Reason == ReasonNotAssigned {
				r.Skipped[i].Reason = reason
				r.Skipped[i].Until = until
			}
			return
		}
	}
	r.Skipped = append(r.Skipped, Skipped{MeteringPoint: meter, Reason: reason, Until: until})
}

func (r *Run) dropSkipped(meter string) {
	kept := r.Skipped[:0]
	for _, s := range r.Skipped {
		if s.MeteringPoint != meter {
			kept = append(kept, s)
		}
	}
	r.Skipped = kept
}

// Save stores the run before the requests are sent, so every answer has a higher row id. A run that sends nothing
// (everything skipped) is not stored: the caller reports the skipped metering points. A request with fewer than two
// metering points gets no summary; it is still stored when it sends a request, so its answer is not counted for an
// older open run. Errors are logged: the request itself goes on.
func Save(ctx context.Context, store Store, tenant string, requested []string, r *Run) {
	distinct := map[string]bool{}
	for _, m := range requested {
		if !distinct[m] {
			distinct[m] = true
			r.Skip(m, ReasonNotFound) // no-op for metering points that were found (requested or skipped)
		}
	}
	if len(r.Meters) == 0 {
		return
	}
	if len(distinct) < 2 {
		r.NoSummary = true
	}
	r.CreatedAt = time.Now().UnixMilli()
	if _, err := store.SaveEnergyRequestRow(ctx, tenant, RowTypeRun, r); err != nil {
		logrus.WithField("tenant", tenant).WithError(err).Error("energy request summary: cannot save run")
	}
}

// Start evaluates open runs now and then every CheckInterval until ctx ends.
func Start(ctx context.Context, store Store) {
	go func() {
		ticker := time.NewTicker(CheckInterval)
		defer ticker.Stop()
		for {
			if err := Check(ctx, store, time.Now()); err != nil {
				logrus.WithError(err).Error("energy request summary: check failed")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Check evaluates the open runs and writes the summaries of the finished ones. It reads the rows of the last LookBack
// of all tenants and attributes the answers per community, as the lock and the live entry do on the rows of one
// community.
func Check(ctx context.Context, store Store, now time.Time) error {
	checkMu.Lock()
	defer checkMu.Unlock()

	rows, err := store.EnergyRequestRows(ctx, nil)
	if err != nil {
		return err
	}

	closed := summaries(rows)
	tenantsByEcId := map[string]map[string]bool{}
	attributions := map[string]attribution{}
	for _, r := range rows {
		if r.Type != RowTypeRun {
			continue
		}
		var run Run
		if err := json.Unmarshal(r.Message, &run); err != nil || closed[run.RunId] != 0 || run.NoSummary {
			continue
		}
		ecId := strings.ToUpper(run.EcId)
		tenants, ok := tenantsByEcId[ecId]
		if !ok {
			list, err := store.TenantsByEcId(ctx, run.EcId)
			if err != nil {
				return err
			}
			tenants = map[string]bool{r.Tenant: true}
			for _, t := range list {
				tenants[t] = true
			}
			tenantsByEcId[ecId] = tenants
			attributions[ecId] = attribute(run.EcId, rows, tenants)
		}

		summary, done := evaluate(run, r.Id, attributions[ecId], now)
		if !done {
			continue
		}
		if _, err := store.CloseEnergyRequest(ctx, r.Tenant, r.Id, summary); err != nil {
			return err
		}
	}
	return nil
}
