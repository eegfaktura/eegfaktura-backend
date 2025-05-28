package services

import (
	protobuf "at.ourproject/vfeeg-backend/proto"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"net"
)

func StartGRPCServer(quit chan bool) {
	port := viper.GetInt("grpc-provider.port")
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}
	defer func() {
		listener.Close()
		log.Info("gRPC Server stops")
	}()
	log.Infof("gRPC Server listen on %s", fmt.Sprintf(":%d", port))

	grpcServer := grpc.NewServer()
	protobuf.RegisterRegisterEegServiceServer(grpcServer, &RegisterService{})
	protobuf.RegisterApiServiceServer(grpcServer, &ApiService{})
	protobuf.RegisterAdminEegServiceServer(grpcServer, &AdminService{})

	go func() {
		<-quit
		grpcServer.GracefulStop()
	}()

	err = grpcServer.Serve(listener)
	if err != nil {
		log.Printf("gRPC: Serve() error: %s", err)
	}
	log.Println("gRPC listener stopped")
}
