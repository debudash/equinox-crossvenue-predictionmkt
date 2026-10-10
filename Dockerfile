# Build: pure Go (modernc SQLite needs no cgo), so the binary is static.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/equinox ./cmd/equinox

# Run: the binary, config and the start script. Data lives in /app/data (mount a volume).
# Alpine rather than distroless so the start script can run watch and serve side by side.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/equinox /app/equinox
COPY config /app/config
COPY scripts/docker/start.sh /app/start.sh
# Runs as root: hosted volumes (Railway) mount root-owned.
RUN chmod +x /app/start.sh && mkdir -p /app/data
VOLUME /app/data
EXPOSE 8080
# Default: watch plus serve (hosted). Override the command to run one CLI step,
# e.g. `docker run ... equinox serve --addr :8080`.
ENTRYPOINT []
CMD ["/app/start.sh"]
