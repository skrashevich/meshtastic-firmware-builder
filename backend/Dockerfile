FROM golang:1.22-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM docker:27-cli

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY --from=build /out/server /usr/local/bin/server

EXPOSE 8080

ENTRYPOINT ["server"]
