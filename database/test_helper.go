package database

import (
	"context"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"io/ioutil"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // used by migrator
	_ "github.com/golang-migrate/migrate/v4/source/file"       // used by migrator
	//"github.com/jackc/pgx/v4/pgxpool"
	//_ "github.com/jackc/pgx/v4/stdlib" // used by migrator
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
	connectionString string
	container        testcontainers.Container
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
		container:        container,
		connectionString: connectionString,
		DbAddress:        dbAddr,
	}
}

func (tdb *TestDatabase) Open() {
	tdb.DbInstance, _ = sqlx.Open("postgres", tdb.connectionString)
}

func (tdb *TestDatabase) TearDown() {
	tdb.DbInstance.Close()
	// remove test container
	_ = tdb.container.Terminate(context.Background())
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
			Image:        "postgres:15-alpine",
			ExposedPorts: []string{port},
			Env:          env,
			WaitingFor:   wait.ForLog("database system is ready to accept connections"),
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
	pathToMigrationFiles := filepath.Dir(path) + "/../migrations"

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
	file, err := ioutil.ReadFile(filepath.Dir(path) + "/migration/000002_init-setup.up.sql")
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	db, err := sqlx.Open("postgres", connectionStr)
	if err != nil {
		return err
	}
	defer db.Close()

	// Execute all
	_, err = db.Exec(string(file))
	if err != nil {
		fmt.Println(err.Error())
	}
	return err
}
