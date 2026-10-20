package database

import (
	"context"
	"encoding/json"

	"at.ourproject/vfeeg-backend/energyrequest"
	"at.ourproject/vfeeg-backend/model"
)

// EnergyRequestRepository keeps the energy request summary (platform#173) in base.notification: no own tables. All
// reads are id ranges on the primary key, the table has no other index.
type EnergyRequestRepository interface {
	SaveEnergyRequestRow(ctx context.Context, tenant, rowType string, value interface{}) (int64, error)
	EnergyRequestLowerBound(ctx context.Context) (int64, error)
	EnergyRequestRowsSince(ctx context.Context, fromId int64) ([]energyrequest.Row, error)
	TenantsByEcId(ctx context.Context, ecId string) ([]string, error)
	CloseEnergyRequest(ctx context.Context, tenant string, runRowId int64, summary energyrequest.Summary) (bool, error)
}

// energyRequestLock serialises closing runs (pg_advisory_xact_lock), so a summary is written once.
const energyRequestLock = 173173

// SaveEnergyRequestRow writes a hidden row (process ENERGY_REQUEST, not listed in Messages) and returns its id.
func (db *sqlDatabase) SaveEnergyRequestRow(ctx context.Context, tenant, rowType string, value interface{}) (int64, error) {
	msg, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	var id int64
	err = db.db.GetContext(ctx, &id, `INSERT INTO base.notification (tenant, notification, type, role, process, date)
		VALUES ($1, $2, $3, 'ADMIN', $4, now() AT TIME ZONE 'utc') RETURNING id`,
		tenant, string(msg), rowType, model.N_PROCESS_ENERGY_REQUEST)
	return id, err
}

// EnergyRequestLowerBound returns the id of the oldest run that can still be open (younger than the deadline plus a
// margin), or the next id when there is none. It only looks at the newest 500,000 rows (about 13 days at 37,000 rows
// a day in prod, 20.10.2026), so the primary key bounds the scan.
func (db *sqlDatabase) EnergyRequestLowerBound(ctx context.Context) (int64, error) {
	var id int64
	err := db.db.GetContext(ctx, &id, `WITH m AS (SELECT COALESCE(max(id), 0) AS max_id FROM base.notification)
		SELECT COALESCE(min(n.id), (SELECT max_id FROM m) + 1) FROM base.notification n
		WHERE n.id > (SELECT max_id FROM m) - 500000 AND n.process = $1 AND n.type = $2
		  AND n.date > (now() AT TIME ZONE 'utc') - interval '25 hours'`,
		model.N_PROCESS_ENERGY_REQUEST, energyrequest.RowTypeRun)
	return id, err
}

// EnergyRequestRowsSince returns the rows the evaluation needs from fromId on: the hidden run rows and the answers
// (ANTWORT_PT, ABLEHNUNG_PT) and summaries among the EDA notifications.
func (db *sqlDatabase) EnergyRequestRowsSince(ctx context.Context, fromId int64) ([]energyrequest.Row, error) {
	rows := []energyrequest.Row{}
	err := db.db.SelectContext(ctx, &rows, `SELECT id, tenant, process, type, notification::text AS notification
		FROM base.notification
		WHERE id >= $1 AND (process = $2
		   OR (process = $3 AND notification->>'type' IN ('ANTWORT_PT', 'ABLEHNUNG_PT', $4)))
		ORDER BY id`,
		fromId, model.N_PROCESS_ENERGY_REQUEST, model.N_PROCESS_EDA_PROCESS, energyrequest.SummaryType)
	return rows, err
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
	if _, err = tx.ExecContext(ctx, `INSERT INTO base.notification (tenant, notification, type, role, process, date)
		VALUES ($1, $2, $3, 'ADMIN', $4, now() AT TIME ZONE 'utc')`,
		tenant, string(msg), model.N_TYPE_MESSAGE, model.N_PROCESS_EDA_PROCESS); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
