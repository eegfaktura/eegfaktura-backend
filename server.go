package main

import (
	"at.ourproject/vfeeg-backend/api"
	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/config"
	"at.ourproject/vfeeg-backend/eda"
	"at.ourproject/vfeeg-backend/graph"
	"at.ourproject/vfeeg-backend/graph/generated"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/util"
	"flag"
	"fmt"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net/http"
	"time"
)

func InitRouters() *mux.Router {

	jwtWrapper := middleware.JWTMiddleware(viper.GetString("jwt.pubKeyFile"))

	//r := mux.NewRouter().PathPrefix("/api").Subrouter()
	r := mux.NewRouter()
	s := r.PathPrefix("/").Subrouter()
	s = api.InitEegRouter(s, jwtWrapper)
	s = api.InitParticipantRouter(s, jwtWrapper)
	s = api.InitMeteringRouter(s, jwtWrapper)
	s = api.InitProcessRouter(s, jwtWrapper)

	return s
}

func main() {
	var configPath = flag.String("configPath", ".", "Configfile Path")
	flag.Parse()
	config.ReadConfig(*configPath)

	err := mqttclient.StartMessageBroker()
	if err != nil {
		panic(err)
	}

	log.SetReportCaller(true)

	eda.InitEdaSubscription()
	mqttclient.InitErrorSubscriptions()

	gqlSrv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{}}))
	r := InitRouters()
	r.Handle("/query", gqlSrv)
	r.Use(middleware.GQLMiddleware(viper.GetString("jwt.pubKeyFile")))

	//messageBroker.Subscribe(mqttclient.GetSubsriptions()...)

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

	go util.StartGRPCServer()

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
	log.Fatal(srv.ListenAndServe())
}
