# Both binaries come out of one build, so the image that serves a schema is the image that applied
# it. Documentation is committed, not generated here — the build stage carries no swag toolchain.
FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/api . && \
    CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

FROM alpine:3.22

# DB_SSLMODE defaults to require, so the runtime image needs the CA bundle to verify the database's
# certificate. Local compose is the one place TLS is turned down.
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app

COPY --from=build /out/api /out/migrate /usr/local/bin/

USER app
EXPOSE 8080
ENTRYPOINT ["api"]
