package database

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"at.ourproject/vfeeg-backend/energyrequest"
	"at.ourproject/vfeeg-backend/model"
	"github.com/jmoiron/sqlx"
)

// EnergyRequestRepository keeps the energy request summary (platform#173) in base.notification: no own tables. All
// reads are id ranges on the primary key, the table has no other index.
type EnergyRequestRepository interface {
	SaveEnergyRequestRow(ctx context.Context, tenant, rowType string, value interface{}) (int64, error)
	UpdateEnergyRequestRow(ctx context.Context, id int64, value interface{}) error
	DeleteEnergyRequestRow(ctx context.Context, id int64) error
	EnergyRequestRows(ctx context.Context, tenants []string) ([]energyrequest.Row, error)
	EnergyRequestCommunity(ctx context.Context, tenant string) ([]energyrequest.Row, map[string]bool, error)
	TenantsByEcId(ctx context.Context, ecId string) ([]string, error)
	CloseEnergyRequest(ctx context.Context, tenant string, runRowId int64, summary energyrequest.Summary) (bool, error)
}

// communityCacheTTL is how long the notification list keeps the energy request rows of a tenant's community.
const communityCacheTTL = 15 * time.Second

// communityCache keeps EnergyRequestCommunity for the notification list, per tenant and per instance: web polls the
// list while a live entry is shown. It is cleared when this instance writes or changes a run or writes a summary;
// answers arriving meanwhile show up after communityCacheTTL at the latest. The lock reads uncached.
var communityCache = struct {
	sync.Mutex
	entries map[string]communityRows
}{entries: map[string]communityRows{}}

type communityRows struct {
	at      time.Time
	rows    []energyrequest.Row
	tenants map[string]bool
}

func (db *sqlDatabase) energyRequestCommunityCached(ctx context.Context, tenant string) ([]energyrequest.Row, map[string]bool, error) {
	communityCache.Lock()
	e, ok := communityCache.entries[tenant]
	communityCache.Unlock()
	if ok && time.Since(e.at) < communityCacheTTL {
		return e.rows, e.tenants, nil
	}
	rows, tenants, err := db.EnergyRequestCommunity(ctx, tenant)
	if err != nil {
		return nil, nil, err
	}
	communityCache.Lock()
	communityCache.entries[tenant] = communityRows{at: time.Now(), rows: rows, tenants: tenants}
	communityCache.Unlock()
	return rows, tenants, nil
}

func clearCommunityCache() {
	communityCache.Lock()
	communityCache.entries = map[string]communityRows{}
	communityCache.Unlock()
}

// energyRequestLock serialises closing runs (pg_advisory_xact_lock), so a summary is written once.
const energyRequestLock = 173173

// SaveEnergyRequestRow writes a hidden row (process ENERGY_REQUEST, not listed in Messages) and returns its id.
func (db *sqlDatabase) SaveEnergyRequestRow(ctx context.Context, tenant, rowType string, value interface{}) (int64, error) {
	msg, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	defer clearCommunityCache()
	var id int64
	err = db.db.GetContext(ctx, &id, `INSERT INTO base.notification (tenant, notification, type, role, process, date)
		VALUES ($1, $2, $3, 'ADMIN', $4, now() AT TIME ZONE 'utc') RETURNING id`,
		tenant, string(msg), rowType, model.N_PROCESS_ENERGY_REQUEST)
	return id, err
}

// UpdateEnergyRequestRow replaces the content of a hidden row.
func (db *sqlDatabase) UpdateEnergyRequestRow(ctx context.Context, id int64, value interface{}) error {
	msg, err := json.Marshal(value)
	if err != nil {
		return err
	}
	defer clearCommunityCache()
	_, err = db.db.ExecContext(ctx, `UPDATE base.notification SET notification = $1 WHERE id = $2 AND process = $3`,
		string(msg), id, model.N_PROCESS_ENERGY_REQUEST)
	return err
}

// DeleteEnergyRequestRow deletes a hidden row.
func (db *sqlDatabase) DeleteEnergyRequestRow(ctx context.Context, id int64) error {
	defer clearCommunityCache()
	_, err := db.db.ExecContext(ctx, `DELETE FROM base.notification WHERE id = $1 AND process = $2`,
		id, model.N_PROCESS_ENERGY_REQUEST)
	return err
}

// EnergyRequestRows returns the rows every evaluation needs from energyrequest.LookBack on (ascending id): the hidden
// run rows and the answers (ANTWORT_PT, ABLEHNUNG_PT) and summaries among the EDA notifications; of the given tenants,
// or of all tenants for nil. The lower id bound is the newest row older than LookBack: the primary key is read
// backwards up to it, so only the rows of that window are scanned (date grows with id; both are set on insert).
func (db *sqlDatabase) EnergyRequestRows(ctx context.Context, tenants []string) ([]energyrequest.Row, error) {
	query := `WITH b AS (SELECT COALESCE((SELECT id FROM base.notification
			WHERE date <= (now() AT TIME ZONE 'utc') - make_interval(secs => ?) ORDER BY id DESC LIMIT 1), 0) AS id)
		SELECT id, tenant, process, type, notification::text AS notification
		FROM base.notification
		WHERE id > (SELECT id FROM b) AND ((process = ? AND type = ?)
		   OR (process = ? AND notification->>'type' IN ('ANTWORT_PT', 'ABLEHNUNG_PT', ?)))`
	args := []interface{}{energyrequest.LookBack.Seconds(), model.N_PROCESS_ENERGY_REQUEST, energyrequest.RowTypeRun,
		model.N_PROCESS_EDA_PROCESS, energyrequest.SummaryType}
	if tenants != nil {
		if len(tenants) == 0 {
			return []energyrequest.Row{}, nil
		}
		query += ` AND tenant IN (?)`
		args = append(args, tenants)
	}
	query, args, err := sqlx.In(query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	rows := []energyrequest.Row{}
	err = db.db.SelectContext(ctx, &rows, db.db.Rebind(query), args...)
	return rows, err
}

// EnergyRequestCommunity returns the rows of EnergyRequestRows for the tenants of tenant's community id (several for
// a GEA), and these tenants (tenant included).
func (db *sqlDatabase) EnergyRequestCommunity(ctx context.Context, tenant string) ([]energyrequest.Row, map[string]bool, error) {
	list := []string{}
	if err := db.db.SelectContext(ctx, &list, `SELECT tenant FROM base.eeg
		WHERE "communityId" = (SELECT "communityId" FROM base.eeg WHERE tenant = $1)`, tenant); err != nil {
		return nil, nil, err
	}
	tenants := map[string]bool{tenant: true}
	for _, t := range list {
		tenants[t] = true
	}
	list = list[:0]
	for t := range tenants {
		list = append(list, t)
	}
	rows, err := db.EnergyRequestRows(ctx, list)
	return rows, tenants, err
}

// TenantsByEcId returns the tenants of a community id (several for a GEA).
func (db *sqlDatabase) TenantsByEcId(ctx context.Context, ecId string) ([]string, error) {
	tenants := []string{}
	err := db.db.SelectContext(ctx, &tenants, `SELECT tenant FROM base.eeg WHERE "communityId" = $1`, ecId)
	return tenants, err
}

// CloseEnergyRequest writes the summary of the run in row runRowId, unless one was written already. Check and insert
// run under a transaction-scoped advisory lock: no second summary from an overlapping check or a second instance.
func (db *sqlDatabase) CloseEnergyRequest(ctx context.Context, tenant string, runRowId int64, summary energyrequest.Summary) (bool, error) {
	msg, err := json.Marshal(summary)
	if err != nil {
		return false, err
	}
	tx, err := db.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, energyRequestLock); err != nil {
		return false, err
	}
	var exists bool
	if err = tx.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM base.notification
		WHERE id > $1 AND process = $2 AND notification->>'type' = $3 AND notification->>'runId' = $4)`,
		runRowId, model.N_PROCESS_EDA_PROCESS, energyrequest.SummaryType, summary.RunId); err != nil {
		return false, err
	}
	if exists {
		return false, tx.Commit()
	}
	defer clearCommunityCache()
	if _, err = tx.ExecContext(ctx, `INSERT INTO base.notification (tenant, notification, type, role, process, date)
		VALUES ($1, $2, $3, 'ADMIN', $4, now() AT TIME ZONE 'utc')`,
		tenant, string(msg), model.N_TYPE_MESSAGE, model.N_PROCESS_EDA_PROCESS); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
