package cmd

import (
	"at.ourproject/vfeeg-backend/api"
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/eda"
	"at.ourproject/vfeeg-backend/graph"
	"at.ourproject/vfeeg-backend/graph/generated"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/repository"
	"at.ourproject/vfeeg-backend/services"
	"context"
	"errors"
	"fmt"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func init() {
	RootCmd.AddCommand(serverCmd)
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts the master data server",
	Long:  "This subcommand starts the master data server.",
	RunE:  handleServer,
}

func handleServer(cmd *cobra.Command, args []string) error {
	middleware.InitKeycloak()
	broker, err := mqttclient.Broker().Init(mqttclient.NewMqttClient)
	if err != nil {
		panic(err)
	}
	eda.InitEdaSubscription()
	mqttclient.InitErrorSubscriptions()

	quit := captureOsInterrupt()
	r := initRouters()

	gqlSrv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{}}))
	r.Handle("/query", middleware.GQLProtect(gqlSrv))

	allowedOrigins := handlers.AllowedOrigins([]string{"*"})
	allowedHeaders := handlers.AllowedHeaders(
		[]string{"X-Requested-With",
			"Accept",
			"Accept-Encoding",
			"Accept-Language",
			"Host",
			"authorization",
			"Content-Type",
			"Content-Length",
			"X-Content-Type-Options",
			"Origin",
			"Connection",
			"Referer",
			"User-Agent",
			"Sec-Fetch-Dest",
			"Sec-Fetch-Mode",
			"Sec-Fetch-Site",
			"Cache-Control",
			"tenant",
			"X-tenant"})
	//allowedHeaders := handlers.AllowedHeaders(
	//	[]string{"authorization", "content-type"})
	allowedMethods := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS", "DELETE"})
	allowedCredentials := handlers.AllowCredentials()

	repository.InitRepositories()
	go services.StartGRPCServer(quit)

	log.Infof("VFEEG BACKEND Config:  host: %s  port: %d  database:%s  user:%s",
		viper.GetString("database.host"),
		viper.GetInt("database.port"),
		viper.GetString("database.dbname"),
		viper.GetString("database.user"))
	log.Infof("VFEEG BACKEND is going to listen on %s", fmt.Sprintf("127.0.0.1:%d", viper.GetInt("port")))

	srv := &http.Server{
		Handler: handlers.CORS(allowedOrigins, allowedHeaders, allowedMethods, allowedCredentials)(r),
		Addr:    fmt.Sprintf("0.0.0.0:%d", viper.GetInt("port")),
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 180 * time.Second,
		ReadTimeout:  180 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen and serve returned err: %v", err)
		}
	}()

	<-quit
	log.Println("got interruption signal")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("server shutdown returned an err: %v\n", err)
	}

	broker.Stop()
	repository.CloseRepositories()
	log.Println("final")

	fmt.Println("STOP PROGRAM")
	return nil
}

func captureOsInterrupt() chan bool {
	quit := make(chan bool)
	go func() {
		c := make(chan os.Signal, 2)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		for sig := range c {
			log.Infof("captured %v, stopping and exiting.", sig)

			quit <- true
			close(quit)

			break
		}
	}()
	return quit
}

func initRouters() *mux.Router {

	//middleware.InitKeycloak()

	//r := mux.NewRouter().PathPrefix("/api").Subrouter()
	r := mux.NewRouter()
	s := r.PathPrefix("/").Subrouter()
	s = api.InitEegRouter(s)
	s = api.InitParticipantRouter(s)
	s = api.InitMeteringRouter(s)
	s = api.InitProcessRouter(s)
	s = api.InitApiRouter(s)

	return s
}
