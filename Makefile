# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=vfeeg-backend
DOCKER=docker
VERSION=v0.1.0

GOPATH := ${PWD}/..:${GOPATH}
export GOPATH

all: test build
build:
	$(GOBUILD) -o $(BINARY_NAME) -v -ldflags="-s -w"
test:
	$(GOTEST) -v ./...
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

docker-clean:
	$(DOCKER) rmi ghcr.io/vfeeg-development/vfeeg-backend:$(VERSION)

docker:
	$(DOCKER) build -t ghcr.io/vfeeg-development/vfeeg-backend:$(VERSION) .

push: docker
	$(DOCKER) push ghcr.io/vfeeg-development/vfeeg-backend:$(VERSION)

protoc:
	protoc --experimental_allow_proto3_optional=true --proto_path=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative ./proto/*.proto
