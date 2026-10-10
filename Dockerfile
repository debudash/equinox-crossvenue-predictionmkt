# Build: pure Go (modernc SQLite needs no cgo), so the binary is static.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/equinox ./cmd/equinox

# Run: the binary and config. Data lives in /app/data (mount a volume).
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/equinox /app/equinox
COPY config /app/config
VOLUME /app/data
EXPOSE 8080
ENTRYPOINT ["/app/equinox"]
CMD ["serve", "--addr", ":8080"]
