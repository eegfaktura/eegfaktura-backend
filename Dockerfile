FROM golang:1.20

ENV TZ="Europe/Berlin"

WORKDIR /usr/src/app

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -o /usr/local/bin/vfeeg-backend -ldflags="-s -w" server.go

COPY zertifikat-pub.pem /usr/local/bin/
COPY config.yaml /etc/backend/

VOLUME /opt/public

RUN rm -r ./*

EXPOSE 8080

CMD ["vfeeg-backend", "-configPath", "/etc/backend/"]