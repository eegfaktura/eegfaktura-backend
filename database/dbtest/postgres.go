// Package dbtest starts the PostgreSQL container of the database tests, with the migrations and the
// seed data. Only tests import it, so Testcontainers and the Docker client stay out of the service
// binary. It does not import package database, so the tests inside that package can use it too.
package dbtest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // used by migrator
	_ "github.com/golang-migrate/migrate/v4/source/file"       // used by migrator
	"github.com/jmoiron/sqlx"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	DbName = "test_db"
	DbUser = "test_user"
	DbPass = "test_password"
)

type TestDatabase struct {
	DbAddress        string
	DbInstance       *sqlx.DB
	ConnectionString string
	Container        testcontainers.Container
}

func SetupTestDatabase() *TestDatabase {

	// setup db container
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	container, connectionString, dbAddr, err := createContainer(ctx)
	if err != nil {
		log.Fatal("failed to setup test", err)
	}

	// migrate db schema
	err = migrateDb(dbAddr)
	if err != nil {
		log.Fatal("failed to perform db migration", err)
	}
	cancel()

	return &TestDatabase{
		Container:        container,
		ConnectionString: connectionString,
		DbAddress:        dbAddr,
	}
}

func (tdb *TestDatabase) Open(ctx context.Context) error {
	var err error
	tdb.DbInstance, err = sqlx.ConnectContext(ctx, "postgres", tdb.ConnectionString)
	return err
}

func (tdb *TestDatabase) TearDown() {
	tdb.DbInstance.Close()
	// remove test container
	_ = tdb.Container.Terminate(context.Background())
}

func createContainer(ctx context.Context) (testcontainers.Container, string, string, error) {

	var env = map[string]string{
		"POSTGRES_PASSWORD": DbPass,
		"POSTGRES_USER":     DbUser,
		"POSTGRES_DB":       DbName,
		//"TZ":                "Europe/Berlin",
		//"PGTZ":              "Europe/Berlin",
	}
	var port = "5432/tcp"

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18.6-alpine", // the zones run PostgreSQL 18
			ExposedPorts: []string{port},
			Env:          env,
			// The message appears twice: first from the temporary server of initdb, then from
			// the real server.
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		},
		Started: true,
	}
	container, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return container, "", "", fmt.Errorf("failed to start container: %v", err)
	}

	p, err := container.MappedPort(ctx, "5432")
	if err != nil {
		return container, "", "", fmt.Errorf("failed to get container external port: %v", err)
	}

	log.Println("postgres container ready and running at port: ", p.Port())

	time.Sleep(time.Second)

	dbAddr := fmt.Sprintf("localhost:%s", p.Port())

	//db, err := sqlx.Open("postgres", fmt.Sprintf("host=localhost port=%s user=%s password=%s dbname=%s sslmode=disable", p.Port(), DbUser, DbPass, DbName))
	//if err != nil {
	//	return container, "", dbAddr, fmt.Errorf("failed to establish database connection: %v", err)
	//}

	return container, fmt.Sprintf("host=localhost port=%s user=%s password=%s dbname=%s sslmode=disable", p.Port(), DbUser, DbPass, DbName), dbAddr, nil
}

func migrateDb(dbAddr string) error {

	// get location of test
	_, path, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("failed to get path")
	}
	pathToMigrationFiles := filepath.Dir(path) + "/../../migrations"

	databaseURL := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", DbUser, DbPass, dbAddr, DbName)
	m, err := migrate.New(fmt.Sprintf("file:%s", pathToMigrationFiles), databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	log.Println("migration done")
	return loadSqlFile(databaseURL, path)
}

func loadSqlFile(connectionStr, path string) error {
	// Read file
	file, err := os.ReadFile(filepath.Dir(path) + "/../migration/000002_init-setup.up.sql")
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	dbm, err := sqlx.Open("postgres", connectionStr)
	if err != nil {
		return err
	}
	defer dbm.Close()

	// Execute all
	_, err = dbm.Exec(string(file))
	if err != nil {
		fmt.Println(err.Error())
	}
	return err
}
