# VFEEG Backend

The VFEEG (Verein zur Förderung von erneuerbaren Energiegemeinschaften) Backend is a Go-based service designed for energy management, participant tracking, and metering point operations. It provides a robust API (GraphQL and REST) to interact with energy data, integrates with MQTT for real-time messaging, and uses PostgreSQL for data persistence.

Part of the **eegfaktura** suite — an open-source billing and management platform
for Austrian renewable energy communities (*Erneuerbare-Energiegemeinschaften*, EEG).
It is the core domain service: it owns EEG, participant and metering-point data, and
consumes EDA market messages relayed from `eegfaktura-eda-xp` over MQTT.

**Tech stack:** Go · Gorilla mux (REST) · gqlgen (GraphQL) · gRPC · goqu ·
golang-migrate + Atlas · Eclipse Paho (MQTT) · Keycloak/OIDC (JWT) · Viper · logrus.
**Exposed ports:** HTTP (default `9080`, `8080` in container) and gRPC (`9092`).
**Talks to:** PostgreSQL, an MQTT broker (mosquitto), Keycloak, and `eegfaktura-eda-xp`.

## Features

- **GraphQL API**: Flexible data querying and manipulation using `gqlgen`.
- **REST API**: Standard endpoints for EEG, participants, metering, and processes.
- **MQTT Integration**: Real-time message handling and error subscriptions.
- **gRPC Support**: Internal service-to-service communication.
- **Database Migrations**: Automated schema management using `golang-migrate` and `atlas`.
- **Dockerized**: Easy deployment using Docker and multi-stage builds.
- **Keycloak Integration**: Secure authentication and authorization.

## Prerequisites

- **Go**: version 1.25 or higher.
- **PostgreSQL**: for data storage.
- **MQTT Broker**: (e.g., Mosquitto) for real-time messaging.
- **Protoc**: Protocol Buffers compiler (if modifying `.proto` files).

## Getting Started

### Installation

1. Clone the repository.
2. Download dependencies:
   ```bash
   go mod download
   ```

### Configuration

The application uses `config.yaml` for configuration. You can find a template in the root directory.

Key configuration options:
- `port`: HTTP server port (default: 9080).
- `database`: Connection details for PostgreSQL.
- `mqtt`: Connection details for the MQTT broker.
- `grpc-provider`: Port for the gRPC server.

### Building

You can build the project using the provided `Makefile`:

```bash
make build
```

Or manually:

```bash
go build -o vfeeg-backend server.go
```

### Running

To run the application:

```bash
./vfeeg-backend -configPath .
```

Alternatively, use the `run` target in the `Makefile`:

```bash
make run
```

### Testing

Run the test suite using:

```bash
make test
```

## Development

### GraphQL

The project uses `gqlgen`. If you change the GraphQL schema (`graph/*.graphqls`), regenerate the code:

```bash
go generate ./...
```

### Protobuf/gRPC

If you modify files in the `proto/` directory, regenerate the Go code:

```bash
make protoc
```

### Database Migrations

Migrations are stored in the `migrations/` directory. The application automatically applies migrations on startup.

For schema diffing and hashing:
```bash
make altas-hash
make altas-migrage
```

## Docker

Build the Docker image:

```bash
make docker
```

Run with Docker:

```bash
docker run -p 9080:8080 -v ./config.yaml:/etc/backend/config.yaml vfeeg-backend
```

## License

The eegfaktura application suite is open source under the GNU Affero General Public
License v3.0 (AGPL-3.0). See the [eegfaktura organisation](https://github.com/eegfaktura)
for the licensing applicable to this component.
