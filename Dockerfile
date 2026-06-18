FROM golang:1.25 AS builder

RUN apt update && apt install -y protobuf-compiler
WORKDIR /usr/src/app

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
RUN go install github.com/atombender/go-jsonschema@latest

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN go mod tidy

ENV PATH="/go/bin:${PATH}"

# Codegen muss VOR `go install ./...` / `go build` laufen: `make protoc`
# erzeugt das protobuf-Package, `go generate` die gqlgen-/jsonschema-Sourcen.
# Sonst scheitert der Compile an `undefined: protobuf.*`.
RUN make protoc
RUN go generate ./...
RUN go install ./...
RUN go build -o /usr/local/bin/vfeeg-backend -ldflags="-s -w" server.go

FROM golang:1.25

ENV TZ="Europe/Berlin"

COPY --from=builder /usr/local/bin/vfeeg-backend /usr/local/bin/vfeeg-backend
COPY config.yaml /etc/backend/

# Copy the entrypoint
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
# Make it executable
RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /usr/src/app
RUN env
#RUN rm -r ./*

VOLUME /opt/storage

EXPOSE 8080

USER 1000

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["vfeeg-backend", "-configPath", "/etc/backend/"]