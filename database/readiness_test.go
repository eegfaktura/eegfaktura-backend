package database

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestPing_answeringDatabase(t *testing.T) {
	require.NoError(t, ping(context.Background(), testDB.connectionString))
}

func TestPing_unreachableDatabaseFailsWithinConnectTimeout(t *testing.T) {
	// TEST-NET-1 address: nothing answers, so only connect_timeout ends the attempt.
	start := time.Now()
	err := ping(context.Background(), "host=192.0.2.1 port=5432 user=x password=x dbname=x sslmode=disable")
	require.Error(t, err)
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestPing_statementTimeoutIsSetByTheDriver(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDB.connectionString+pingParams)
	require.NoError(t, err)
	defer conn.Close(ctx)

	var timeout string
	require.NoError(t, conn.QueryRow(ctx, "SHOW statement_timeout").Scan(&timeout))
	require.Equal(t, "2s", timeout)
}
