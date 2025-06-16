package cmd

import (
	"at.ourproject/vfeeg-backend/database"
	"embed"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/httpfs"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net/http"
)

var (
	//go:embed migrations/*.sql
	migrations embed.FS
)

type driver struct {
	httpfs.PartialDriver
}

func (d *driver) Open(rawURL string) (source.Driver, error) {
	err := d.PartialDriver.Init(http.FS(migrations), "migrations")
	if err != nil {
		return nil, err
	}

	return d, nil
}

func init() {
	RootCmd.AddCommand(migrateCmd)
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate the database",
	Long:  "This subcommand says goodbye to someone in a specific language.",
	RunE:  handleMigration,
}

func handleMigration(cmd *cobra.Command, args []string) error {
	//m, err := migrate.New(
	//	"file://db/migrations",
	//	"postgres://postgres:postgres@localhost:5432/example?sslmode=disable")

	log.Info("Start migration")
	db, err := database.ConnectToDatabase()
	if err != nil {
		log.Fatal(err)
		return err
	}
	defer func() { _ = db.Close() }()

	source.Register("embed", &driver{})

	dbDriver, err := postgres.WithInstance(db.DB, &postgres.Config{SchemaName: "base"})
	if err != nil {
		log.Fatal(err)
		return err
	}

	m, err := migrate.NewWithDatabaseInstance(
		"embed://",
		"postgres", dbDriver)
	if err != nil {
		log.Fatal(err)
		return err
	}
	if err := m.Up(); err != nil {
		log.Fatal(err)
		return err
	}
	return nil
}
