package database

import (
	"context"
	"errors"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

// The transaction tests (BE-CQ-7) run on sqlmock: a failed COMMIT must reach the caller, and an
// error inside the transaction must roll it back, never commit it.

func newTxMock(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return sqlx.NewDb(conn, "mock"), mock
}

var errCommit = errors.New("commit failed")

func TestInTx_commitErrorReachesCaller(t *testing.T) {
	dbx, mock := newTxMock(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errCommit)

	err := inTx(context.Background(), dbx, func(tx *sqlx.Tx) error { return nil })

	assert.ErrorIs(t, err, errCommit)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInTx_errorRollsBackAndKeepsTheError(t *testing.T) {
	dbx, mock := newTxMock(t)
	mock.ExpectBegin()
	mock.ExpectRollback()
	want := model.ErrUpdateMeter(errors.New("boom"))

	err := inTx(context.Background(), dbx, func(tx *sqlx.Tx) error { return want })

	assert.Same(t, want, err, "the error of fn is returned unchanged, so it keeps its code")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAddTariff_commitErrorReachesCaller(t *testing.T) {
	dbx, mock := newTxMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "base"."tariff" (.+)`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errCommit)

	tariff := model.Tariff{Name: "Sepp", BillingPeriod: "monthly", FreeKWh: null.IntFrom(100), CentPerKWh: 12}
	err := AddTariff(context.Background(), dbx, "TE000001", "sepp", &tariff)

	assert.ErrorIs(t, err, errCommit)
	assert.NoError(t, mock.ExpectationsWereMet())
}

var activeMeter = []model.Meter{{MeteringPoint: "AT0030000000000000000000000000001", Direction: "CONSUMPTION", From: 1}}

func TestUpdateActiveMeteringPoints_failedUpdateRollsBack(t *testing.T) {
	dbx, mock := newTxMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE "base"."meteringpoint"`).WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	err := UpdateActiveMeteringPoints(context.Background(), dbx, "TE000001", activeMeter)

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet(), "the transaction must be rolled back, not committed")
}

func TestUpdateActiveMeteringPoints_commitErrorReachesCaller(t *testing.T) {
	dbx, mock := newTxMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE "base"."meteringpoint"`).WillReturnRows(sqlmock.NewRows([]string{"participant_id"}))
	mock.ExpectCommit().WillReturnError(errCommit)

	err := UpdateActiveMeteringPoints(context.Background(), dbx, "TE000001", activeMeter)

	assert.ErrorIs(t, err, errCommit)
	assert.NoError(t, mock.ExpectationsWereMet())
}
