FROM golang:1.21

ENV TZ="Europe/Berlin"

WORKDIR /usr/src/app

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -o /usr/local/bin/vfeeg-backend -ldflags="-s -w" server.go

COPY config.yaml /etc/backend/

# Copy the entrypoint
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
# Make it executable
RUN chmod +x /usr/local/bin/entrypoint.sh

VOLUME /opt/storage

RUN rm -r ./*

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["vfeeg-backend", "-configPath", "/etc/backend/"]