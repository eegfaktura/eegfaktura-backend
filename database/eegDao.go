package database

import (
	"context"
	"fmt"
	"strings"

	"at.ourproject/vfeeg-backend/model"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
)

const TABLE_EEG = "base.eeg"
const TABLE_EEG_ADDRESS = "base.address"

type EegRepository interface {
	GetEegById(ctx context.Context, tenant string) (*model.Eeg, error)
	GetEegByIdForUser(ctx context.Context, tenant string) (*model.Eeg, error)
	GetEegByEcId(ctx context.Context, edId string) (*model.Eeg, error)
	GetTenantsByRcNumber(ctx context.Context, rcNumber string) ([]string, error)
	UpdateEegPartial(ctx context.Context, tenant string, fields map[string]interface{}) error
	GetGridOperators(ctx context.Context) (map[string]string, error)
	FetchTenantsName(ctx context.Context, tenants []string, isSuperUser bool) ([]tenantsNameStruct, error)
	InsertEeg(ctx context.Context, tenant string, eeg *model.Eeg) error
	UpdateOnlineState(ctx context.Context, tenant string, onlineState bool) error
}

func (db *sqlDatabase) GetEegById(ctx context.Context, tenant string) (*model.Eeg, error) {
	return getEegById(ctx, db.db, tenant)
}
func (db *sqlDatabase) GetEegByIdForUser(ctx context.Context, tenant string) (*model.Eeg, error) {
	eeg, err := getEegById(ctx, db.db, tenant)
	if err != nil {
		return nil, err
	}
	eeg.BankName = null.String{}
	eeg.BusinessNr = null.String{}
	eeg.Contact = model.Contact{}
	eeg.Phone = null.String{}
	eeg.TaxNumber = null.String{}
	eeg.VatNumber = null.String{}

	return eeg, nil
}

func (db *sqlDatabase) GetEegByEcId(ctx context.Context, edId string) (*model.Eeg, error) {
	return getEegByEcId(ctx, db.db, edId)
}

// GetTenantsByRcNumber returns the tenants of an RC number (MQTT topic of incoming EDA messages).
// A GEA owns several tenants per RC number (GC100019-001, GC100019-002, …).
func (db *sqlDatabase) GetTenantsByRcNumber(ctx context.Context, rcNumber string) ([]string, error) {
	stmt, args, err := pgDialect.From(TABLE_EEG).Select("tenant").Where(goqu.Or(
		goqu.Func("upper", goqu.I("rcNumber")).Eq(strings.ToUpper(rcNumber)),
		goqu.Func("upper", goqu.C("tenant")).Eq(strings.ToUpper(rcNumber)),
	)).Order(goqu.C("tenant").Asc()).Prepared(true).ToSQL()
	if err != nil {
		return nil, model.ErrGetEeg(err)
	}
	var tenants []string
	if err = db.db.SelectContext(ctx, &tenants, stmt, args...); err != nil {
		return nil, model.ErrGetEeg(err)
	}
	return tenants, nil
}

func (db *sqlDatabase) UpdateEegPartial(ctx context.Context, tenant string, fields map[string]interface{}) error {
	return updateEegPartial(ctx, db.db, tenant, fields)
}

func (db *sqlDatabase) GetGridOperators(ctx context.Context) (map[string]string, error) {
	return getGridOperators(ctx, db.db)
}

func (db *sqlDatabase) FetchTenantsName(ctx context.Context, tenants []string, isSuperUser bool) ([]tenantsNameStruct, error) {
	return fetchTenantsName(ctx, db.db, tenants, isSuperUser)
}

func (db *sqlDatabase) InsertEeg(ctx context.Context, tenant string, eeg *model.Eeg) error {
	return insertEeg(ctx, db.db, tenant, eeg)
}

func (db *sqlDatabase) UpdateOnlineState(ctx context.Context, tenant string, onlineState bool) error {

	stmt, _, err := goqu.Update(TABLE_EEG).
		Set(goqu.Record{"online": onlineState}).
		Where(goqu.Ex{"tenant": goqu.V(tenant)}).
		ToSQL()

	if err != nil {
		return err
	}

	_, err = db.db.ExecContext(ctx, stmt)
	if err != nil {
		return err
	}
	return nil
}

func getEegById(ctx context.Context, tx *sqlx.DB, tenant string) (*model.Eeg, error) {

	var eeg model.Eeg
	stmt, args, err := pgDialect.From(TABLE_EEG).Select(&eeg).Where(goqu.C("tenant").Eq(tenant)).Prepared(true).ToSQL()
	if err != nil {
		return nil, model.ErrGetEeg(err)
	}

	err = tx.GetContext(ctx, &eeg, stmt, args...)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, model.ErrGetEeg(err)
	}
	return &eeg, nil
}

func getEegByEcId(ctx context.Context, tx *sqlx.DB, edId string) (*model.Eeg, error) {

	var eeg model.Eeg
	stmt, args, err := pgDialect.From(TABLE_EEG).Select(&eeg).Where(goqu.C("communityId").Eq(edId)).Prepared(true).ToSQL()
	if err != nil {
		return nil, model.ErrGetEeg(err)
	}

	err = tx.GetContext(ctx, &eeg, stmt, args...)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, model.ErrGetEeg(err)
	}
	return &eeg, nil
}

func insertEeg(ctx context.Context, db *sqlx.DB, tenant string, eeg *model.Eeg) error {

	// Same shared address rule as updateEegPartial — the create path
	// must not be the one write that skips enforcement.
	if eeg.Contact.Email.Valid {
		normalized, err := model.ValidateEmailList(eeg.Contact.Email.String)
		if err != nil {
			return err
		}
		if normalized == "" {
			eeg.Contact.Email = null.String{}
		} else {
			eeg.Contact.Email = null.StringFrom(normalized)
		}
	}

	sql, _, err := pgDialect.Insert(TABLE_EEG).Rows(eeg).OnConflict(goqu.DoNothing()).ToSQL()
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		log.WithField("SQL", "INSERT").Errorf("Stmt: %s", sql)
		return err
	}

	return err
}

// eegProtectedUpdateFields must never be set through the generic EEG update:
// the identity/tenant key and rcNumber/online/createdAt (model: skipupdate),
// plus communityId and tenant, which route incoming EDA messages and identify
// the community. The web round-trips the whole EEG object on save, so these are
// dropped silently rather than rejected.
var eegProtectedUpdateFields = func() map[string]struct{} {
	m := map[string]struct{}{"tenant": {}, "communityId": {}}
	for _, k := range model.SkipUpdateJSONKeys(model.Eeg{}) {
		m[k] = struct{}{}
	}
	return m
}()

func updateEegPartial(ctx context.Context, db *sqlx.DB, tenant string, fields map[string]interface{}) error {
	// Drop write-protected fields so a client cannot set tenant, rcNumber,
	// communityId etc. through this generic update (mass assignment).
	for k := range eegProtectedUpdateFields {
		delete(fields, k)
	}

	// eeg.Email is the recipient of the ZP list mail and the billing CC —
	// enforce the shared address rule before persisting (normalize,
	// reject invalid).
	if raw, ok := fields["email"].(string); ok {
		normalized, err := model.ValidateEmailList(raw)
		if err != nil {
			return err
		}
		if normalized == "" {
			fields["email"] = nil
		} else {
			fields["email"] = normalized
		}
	}

	// Map every key to a known, updatable column. The key ends up as an SQL
	// identifier, which goqu quotes but does not escape, so an unknown key is
	// rejected instead of being passed through verbatim.
	updateRecord := goqu.Record{}
	for k, v := range fields {
		col, ok := model.ResolveFlatUpdateColumn(model.Eeg{}, k)
		if !ok {
			return fmt.Errorf("field %q cannot be updated", k)
		}
		updateRecord[col] = v
	}
	if len(updateRecord) == 0 {
		return nil
	}

	statement, _, err := pgDialect.Update(TABLE_EEG).Set(updateRecord).Where(goqu.Ex{"tenant": goqu.V(tenant)}).ToSQL()
	if err != nil {
		log.WithError(err).Errorf("Update EEG VALUES: %s", statement)
		return err
	}

	_, err = db.ExecContext(ctx, statement)
	return err
}

func getGridOperators(ctx context.Context, db *sqlx.DB) (map[string]string, error) {

	sql, _, err := pgDialect.From("base.gridoperators").ToSQL()

	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var id string
	var name string
	result := map[string]string{}
	for rows.Next() {
		err = rows.Scan(&id, &name)
		if err != nil {
			return nil, err
		}
		result[id] = name
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

type tenantsNameStruct struct {
	Tenant string `json:"tenant" db:"tenant"`
	Name   string `json:"name" db:"name"`
}

func fetchTenantsName(ctx context.Context, db *sqlx.DB, tenants []string, isSuperUser bool) ([]tenantsNameStruct, error) {
	tenantsName := []tenantsNameStruct{}
	selectStmt := pgDialect.From(TABLE_EEG).Select(&tenantsName)
	if !isSuperUser {
		selectStmt = selectStmt.Where(goqu.C("tenant").In(tenants))
	}
	stmt, _, err := selectStmt.ToSQL()
	if err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &tenantsName, stmt); err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, err
	}
	return tenantsName, nil
}
