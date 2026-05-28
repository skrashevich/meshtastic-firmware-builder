FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
ARG APP_VERSION=dev
ARG APP_COMMIT=""
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-w -s \
        -X github.com/skrashevich/meshtastic-firmware-builder/backend/internal/buildinfo.Version=${APP_VERSION} \
        -X github.com/skrashevich/meshtastic-firmware-builder/backend/internal/buildinfo.Commit=${APP_COMMIT}" \
      -o /out/server \
      ./cmd/server

FROM docker:27-cli

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY --from=build /out/server /usr/local/bin/server

EXPOSE 8080

ENTRYPOINT ["server"]
