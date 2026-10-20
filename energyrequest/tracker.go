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

// Store is the part of the database the summary needs (implemented by database.Database).
type Store interface {
	SaveEnergyRequestRow(ctx context.Context, tenant, rowType string, value interface{}) (int64, error)
	EnergyRequestLowerBound(ctx context.Context) (int64, error)
	EnergyRequestRowsSince(ctx context.Context, fromId int64) ([]Row, error)
	TenantsByEcId(ctx context.Context, ecId string) ([]string, error)
	CloseEnergyRequest(ctx context.Context, tenant string, runRowId int64, summary Summary) (bool, error)
}

// tracker holds only derived state, rebuilt from the database on every check: where to start reading and which
// (community, metering point) pairs belong to open runs, so the CR_MSG handler does not query per delivery.
type tracker struct {
	mu         sync.Mutex
	lowerBound int64                      // 0 = not known yet (after a start)
	open       map[string]map[string]bool // ecId -> metering points of open runs
	registered []registration             // runs registered since the last read
}

type registration struct {
	rowId  int64
	ecId   string
	meters []string
}

var current = &tracker{open: map[string]map[string]bool{}}

// NewRun starts collecting a run for a request of the community ecId for the period from..to.
func NewRun(ecId string, from, to int64) *Run {
	return &Run{Type: RowTypeRun, RunId: uuid.New(), EcId: ecId, From: from, To: to,
		Meters: []MeterRange{}, Skipped: []Skipped{}}
}

// Add records a metering point that is requested for from..to. A metering point that occurs twice (several
// participations in the period) is merged to the widest period.
func (r *Run) Add(meter string, from, to int64) {
	for i := range r.Meters {
		if r.Meters[i].MeteringPoint == meter {
			if from < r.Meters[i].From {
				r.Meters[i].From = from
			}
			if to > r.Meters[i].To {
				r.Meters[i].To = to
			}
			r.dropSkipped(meter)
			return
		}
	}
	r.Meters = append(r.Meters, MeterRange{MeteringPoint: meter, From: from, To: to})
	r.dropSkipped(meter)
}

// Skip records a metering point that is not requested, unless it is requested for another participation.
func (r *Run) Skip(meter, reason string) {
	for _, m := range r.Meters {
		if m.MeteringPoint == meter {
			return
		}
	}
	for _, s := range r.Skipped {
		if s.MeteringPoint == meter {
			return
		}
	}
	r.Skipped = append(r.Skipped, Skipped{MeteringPoint: meter, Reason: reason})
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

// Save stores the run before the requests are sent, so every answer has a higher row id. A request with fewer than
// two metering points gets no summary. Errors are logged: the request itself goes on.
func Save(ctx context.Context, store Store, tenant string, requested []string, r *Run) {
	distinct := map[string]bool{}
	for _, m := range requested {
		distinct[m] = true
	}
	if len(distinct) < 2 {
		return
	}
	for m := range distinct {
		r.markNotFound(m)
	}
	r.CreatedAt = time.Now().UnixMilli()
	id, err := store.SaveEnergyRequestRow(ctx, tenant, RowTypeRun, r)
	if err != nil {
		logrus.WithField("tenant", tenant).WithError(err).Error("energy request summary: cannot save run")
		return
	}
	meters := make([]string, 0, len(r.Meters))
	for _, m := range r.Meters {
		meters = append(meters, m.MeteringPoint)
	}
	current.register(registration{rowId: id, ecId: r.EcId, meters: meters})
}

func (r *Run) markNotFound(meter string) {
	for _, m := range r.Meters {
		if m.MeteringPoint == meter {
			return
		}
	}
	r.Skip(meter, ReasonNotFound)
}

// RecordDelivery writes a hidden DATA row when a metering point of an open run gets energy data (from..to over all
// blocks of the message). Deliveries of other metering points are ignored without a database access.
func RecordDelivery(ctx context.Context, store Store, tenant, ecId, meter string, from, to int64) {
	if !current.tracked(ecId, meter) {
		return
	}
	d := Data{Type: RowTypeData, EcId: ecId, MeteringPoint: meter, From: from, To: to}
	if _, err := store.SaveEnergyRequestRow(ctx, tenant, RowTypeData, d); err != nil {
		logrus.WithField("tenant", tenant).WithError(err).Error("energy request summary: cannot save delivery")
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

// Check evaluates the open runs and writes the summaries of the finished ones.
func Check(ctx context.Context, store Store, now time.Time) error {
	return current.check(ctx, store, now)
}

func (t *tracker) register(reg registration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.registered = append(t.registered, reg)
	// the new row id is above every row read so far, so the lower bound already includes it
	t.addOpen(reg.ecId, reg.meters)
}

func (t *tracker) tracked(ecId, meter string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open[strings.ToUpper(ecId)][meter]
}

func (t *tracker) addOpen(ecId string, meters []string) {
	key := strings.ToUpper(ecId)
	if t.open[key] == nil {
		t.open[key] = map[string]bool{}
	}
	for _, m := range meters {
		t.open[key][m] = true
	}
}

func (t *tracker) check(ctx context.Context, store Store, now time.Time) error {
	t.mu.Lock()
	lowerBound := t.lowerBound
	t.mu.Unlock()

	if lowerBound == 0 {
		lb, err := store.EnergyRequestLowerBound(ctx)
		if err != nil {
			return err
		}
		lowerBound = lb
	}

	rows, err := store.EnergyRequestRowsSince(ctx, lowerBound)
	if err != nil {
		return err
	}

	closed := map[string]bool{}
	var maxId int64
	for _, r := range rows {
		if r.Id > maxId {
			maxId = r.Id
		}
		if r.Type == messageRow {
			var s struct {
				Type  string `json:"type"`
				RunId string `json:"runId"`
			}
			if json.Unmarshal(r.Message, &s) == nil && s.Type == SummaryType {
				closed[s.RunId] = true
			}
		}
	}

	tenantsByEcId := map[string]map[string]bool{}
	open := map[string]map[string]bool{}
	next := maxId + 1
	if maxId == 0 {
		next = lowerBound
	}
	for _, r := range rows {
		if r.Type != RowTypeRun {
			continue
		}
		var run Run
		if err := json.Unmarshal(r.Message, &run); err != nil || closed[run.RunId] {
			continue
		}
		tenants, ok := tenantsByEcId[run.EcId]
		if !ok {
			list, err := store.TenantsByEcId(ctx, run.EcId)
			if err != nil {
				return err
			}
			tenants = map[string]bool{r.Tenant: true}
			for _, tn := range list {
				tenants[tn] = true
			}
			tenantsByEcId[run.EcId] = tenants
		}

		summary, done := Evaluate(run, r.Id, rows, tenants, now)
		if done {
			if _, err := store.CloseEnergyRequest(ctx, r.Tenant, r.Id, summary); err != nil {
				return err
			}
			continue
		}
		if r.Id < next {
			next = r.Id
		}
		key := strings.ToUpper(run.EcId)
		if open[key] == nil {
			open[key] = map[string]bool{}
		}
		for _, m := range run.Meters {
			open[key][m.MeteringPoint] = true
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	// runs registered after the rows were read are not in rows: keep them open
	var kept []registration
	for _, reg := range t.registered {
		if reg.rowId > maxId {
			kept = append(kept, reg)
			if reg.rowId < next {
				next = reg.rowId
			}
		}
	}
	t.registered = kept
	t.open = open
	for _, reg := range kept {
		t.addOpen(reg.ecId, reg.meters)
	}
	t.lowerBound = next
	return nil
}
