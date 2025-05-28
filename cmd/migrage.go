package cmd

import (
	"at.ourproject/vfeeg-backend/database"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(migrageCmd)
}

var migrageCmd = &cobra.Command{
	Use:   "goodbye",
	Short: "Say goodbye to someone",
	Long:  "This subcommand says goodbye to someone in a specific language.",
	RunE:  handleMigration,
}

func handleMigration(cmd *cobra.Command, args []string) error {
	//m, err := migrate.New(
	//	"file://db/migrations",
	//	"postgres://postgres:postgres@localhost:5432/example?sslmode=disable")

	db, err := database.ConnectToDatabase()
	if err != nil {
		log.Fatal(err)
		return err
	}
	defer func() { _ = db.Close() }()

	driver, err := postgres.WithInstance(db.DB, &postgres.Config{})
	m, err := migrate.NewWithDatabaseInstance(
		"file:///migrations",
		"postgres", driver)
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
