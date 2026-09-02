# All three binaries come out of one build, so the image that serves a schema is the image that
# applied it and the image that filled it. Documentation is committed, not generated here — the
# build stage carries no swag toolchain.
FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/api . && \
    CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -o /out/ingest ./cmd/ingest

FROM alpine:3.22

# DB_SSLMODE defaults to require, so the runtime image needs the CA bundle to verify the database's
# certificate — and ingest needs it again to verify OpenF1's. Local compose is the one place TLS is
# turned down, and only for the database.
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app

COPY --from=build /out/api /out/migrate /out/ingest /usr/local/bin/

USER app
EXPOSE 8080
ENTRYPOINT ["api"]
