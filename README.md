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

- **Go**: version 1.26 or higher.
- **PostgreSQL**: for data storage.
- **MQTT Broker**: (e.g., Mosquitto) for real-time messaging.
- **Protoc**: Protocol Buffers compiler (the build generates the gRPC code; CI pins 29.3).

## Getting Started

### Installation

1. Clone the repository.
2. Download dependencies:
   ```bash
   go mod download
   ```
3. Generate the gRPC code and the registration model (not tracked; protoc must be on the `PATH`, the
   generators are installed at the versions in `go.mod`):
   ```bash
   make tools generate
   ```

### Configuration

The application uses `config.yaml` for configuration. You can find a template in the root directory.

Key configuration options:
- `port`: HTTP server port (default: 9080).
- `database`: Connection details for PostgreSQL.
- `mqtt`: Connection details for the MQTT broker.
- `grpc-provider`: Port for the gRPC server.

The Keycloak clients are read from the JSON file named in `KEYCLOAK_CONFIG` (default `./keycloak.json`). The
`keycloak.json` in the repository is a sample: provide your own file with the secret of the
`at.ourproject.vfeeg.api` client and never commit it.

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

The tests of `api`, `database` and `eda` start PostgreSQL through Testcontainers (Docker needed), one container per
package. On a shared or slow Docker (e.g. Docker Desktop) run them one package at a time:

```bash
go test -p 1 ./api ./database ./eda
```

## Development

### GraphQL

The project uses `gqlgen`. If you change the GraphQL schema (`graph/*.graphqls`), regenerate the code:

```bash
go generate ./...
```

### Protobuf/gRPC

The Go code of `proto/*.proto` and `model/registration.schema.json` is generated, not tracked. After changing one
of them, regenerate it:

```bash
make generate
```

### Database Migrations

Migrations are stored in the `migrations/` directory. The application automatically applies migrations on startup.

For schema diffing and hashing:
```bash
make atlas-hash
make atlas-diff
```

#### Failed migration (dirty)

The backend migrates before it serves (one replica, `Recreate`): while a migration runs or waits, the API is down.
Every lock wait of a migration ends after 60 s (`lock_timeout`); the backend then exits with
`event.action=db.migrate.failed` and restarts.

golang-migrate marks a version dirty in `base.schema_migrations` before it runs the file and clears the mark after
it. `dirty = true` therefore means: the file was started and did not finish (it failed, waited too long for a lock,
or the pod was stopped). The backend does not start on a dirty version. The migrate CLI is not in the image; recover
by hand with `psql`:

1. `SELECT version, dirty FROM base.schema_migrations;` and read the file of that version in `migrations/`.
2. Find out how much of the file is in place (cases below). Never only reset the version without this step.
3. File fully in place: `UPDATE base.schema_migrations SET version = <that version>, dirty = false;`.
   Not in place: undo what is left of it, then `UPDATE base.schema_migrations SET version = <the version before it>,
   dirty = false;` — the next start runs the file again.
4. Restart the backend and check the line `event.action=db.migrate.done`.

Cases:

| Dirty version | Check | Recovery |
|---|---|---|
| `20261105110000` (view `activeMeteringPartition`) | the statement either ran or not | `version = 20261028130000`: the file runs again (`CREATE OR REPLACE VIEW`) |
| `20261105110100` (index, `CONCURRENTLY`) | `SELECT indisvalid FROM pg_index WHERE indexrelid = to_regclass('base.metering_partition_factor_tenant_idx');` | no row or `false`: `DROP INDEX CONCURRENTLY IF EXISTS base.metering_partition_factor_tenant_idx;`, then `version = 20261105110000` (with `IF NOT EXISTS` an invalid index would stay) |
| `20261105120000` (table `settlement_interval_change`) | `SELECT to_regclass('base.settlement_interval_change');` | `version = 20261105110100`: the file runs again (`IF NOT EXISTS`) |

## Docker

Build the Docker image:

```bash
docker build -t vfeeg-backend .
```

Run with Docker:

```bash
docker run -p 9080:8080 -v ./config.yaml:/etc/backend/config.yaml vfeeg-backend
```

## License

The eegfaktura application suite is open source under the GNU Affero General Public
License v3.0 (AGPL-3.0). See the [eegfaktura organisation](https://github.com/eegfaktura)
for the licensing applicable to this component.
