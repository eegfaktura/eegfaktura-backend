BINARY_NAME=vfeeg-backend

all: test build

build:
	go build -o $(BINARY_NAME) -ldflags="-s -w" server.go
test:
	go test ./...
clean:
	go clean
	rm -f $(BINARY_NAME)
run: build
	./$(BINARY_NAME) -configPath .

protoc:
	protoc --experimental_allow_proto3_optional=true --proto_path=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative ./proto/*.proto

atlas-hash:
	atlas migrate hash --env local

atlas-diff:
	atlas migrate diff --env local
