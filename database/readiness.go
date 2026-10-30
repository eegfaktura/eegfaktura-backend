package database

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// pingParams bound the readiness ping by the driver itself: 2 s to connect,
// 2 s for the statement.
const pingParams = " connect_timeout=2 statement_timeout=2000"

// Ping checks that the configured database answers. It opens one dedicated,
// short-lived connection outside the application pool, so a busy pool does not
// make the service look unready and callers of /readyz cannot exhaust the pool.
func Ping(ctx context.Context) error {
	return ping(ctx, connInfo())
}

func ping(ctx context.Context, info string) error {
	conn, err := pgx.Connect(ctx, info+pingParams)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	var one int
	return conn.QueryRow(ctx, "SELECT 1").Scan(&one)
}
