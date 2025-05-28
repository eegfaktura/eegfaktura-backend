package main

import (
	"at.ourproject/vfeeg-backend/cmd"
	"at.ourproject/vfeeg-backend/config"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
)

func init() {
	lvl, ok := os.LookupEnv("LOG_LEVEL")
	// LOG_LEVEL not set, let's default to debug
	if !ok {
		lvl = "debug"
	}
	// parse string, this is built-in feature of logrus
	ll, err := log.ParseLevel(lvl)
	if err != nil {
		ll = log.DebugLevel
	}
	// set global log level
	log.SetLevel(ll)
}

//func captureOsInterrupt() chan bool {
//	quit := make(chan bool)
//	go func() {
//		c := make(chan os.Signal, 2)
//		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
//
//		for sig := range c {
//			log.Infof("captured %v, stopping and exiting.", sig)
//
//			quit <- true
//			close(quit)
//
//			break
//		}
//	}()
//	return quit
//}
//
//func InitRouters() *mux.Router {
//
//	//middleware.InitKeycloak()
//
//	//r := mux.NewRouter().PathPrefix("/api").Subrouter()
//	r := mux.NewRouter()
//	s := r.PathPrefix("/").Subrouter()
//	s = api.InitEegRouter(s)
//	s = api.InitParticipantRouter(s)
//	s = api.InitMeteringRouter(s)
//	s = api.InitProcessRouter(s)
//	s = api.InitApiRouter(s)
//
//	return s
//}

func main() {
	var configPath = flag.String("configPath", ".", "Configfile Path")
	flag.Parse()
	config.ReadConfig(*configPath)

	log.SetReportCaller(true)

	cmd.Execute()
	fmt.Printf("Program end: %s\n", "now")

	//broker, err := mqttclient.Broker().Init(mqttclient.NewMqttClient)
	//if err != nil {
	//	panic(err)
	//}
	//eda.InitEdaSubscription()
	//mqttclient.InitErrorSubscriptions()
	//
	//quit := captureOsInterrupt()
	//r := InitRouters()
	//
	//gqlSrv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{}}))
	//r.Handle("/query", middleware.GQLProtect(gqlSrv))
	////r.Use(middleware.GQLMiddleware(viper.GetString("jwt.pubKeyFile")))
	////r.Use(middleware.GQLProtect)
	//
	////messageBroker.Subscribe(mqttclient.GetSubsriptions()...)
	//
	//allowedOrigins := handlers.AllowedOrigins([]string{"*"})
	//allowedHeaders := handlers.AllowedHeaders(
	//	[]string{"X-Requested-With",
	//		"Accept",
	//		"Accept-Encoding",
	//		"Accept-Language",
	//		"Host",
	//		"authorization",
	//		"Content-Type",
	//		"Content-Length",
	//		"X-Content-Type-Options",
	//		"Origin",
	//		"Connection",
	//		"Referer",
	//		"User-Agent",
	//		"Sec-Fetch-Dest",
	//		"Sec-Fetch-Mode",
	//		"Sec-Fetch-Site",
	//		"Cache-Control",
	//		"tenant",
	//		"X-tenant"})
	////allowedHeaders := handlers.AllowedHeaders(
	////	[]string{"authorization", "content-type"})
	//allowedMethods := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS", "DELETE"})
	//allowedCredentials := handlers.AllowCredentials()
	//
	//repository.InitRepositories()
	//go services.StartGRPCServer(quit)
	//
	//log.Infof("VFEEG BACKEND Config:  host: %s  port: %d  database:%s  user:%s",
	//	viper.GetString("database.host"),
	//	viper.GetInt("database.port"),
	//	viper.GetString("database.dbname"),
	//	viper.GetString("database.user"))
	//log.Infof("VFEEG BACKEND is going to listen on %s", fmt.Sprintf("127.0.0.1:%d", viper.GetInt("port")))
	//
	//srv := &http.Server{
	//	Handler: handlers.CORS(allowedOrigins, allowedHeaders, allowedMethods, allowedCredentials)(r),
	//	Addr:    fmt.Sprintf("0.0.0.0:%d", viper.GetInt("port")),
	//	// Good practice: enforce timeouts for servers you create!
	//	WriteTimeout: 180 * time.Second,
	//	ReadTimeout:  180 * time.Second,
	//}
	//
	//go func() {
	//	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
	//		log.Fatalf("listen and serve returned err: %v", err)
	//	}
	//}()
	//
	//<-quit
	//log.Println("got interruption signal")
	//if err := srv.Shutdown(context.Background()); err != nil {
	//	log.Printf("server shutdown returned an err: %v\n", err)
	//}
	//
	//broker.Stop()
	//repository.CloseRepositories()
	//log.Println("final")
	//
	//fmt.Println("STOP PROGRAM")
}
