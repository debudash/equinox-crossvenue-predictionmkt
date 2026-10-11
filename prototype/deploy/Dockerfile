# Combined image: Equinox core (watch + serve, unchanged) plus the proposal app (Streamlit) on one volume.
# Build context is the repo root:  docker build -f prototype/deploy/Dockerfile -t equinox-combined .
# On Railway, the service variable RAILWAY_DOCKERFILE_PATH=prototype/deploy/Dockerfile selects this file.

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The core binary exactly as the core Dockerfile builds it, plus the core's synthetic demo seed.
RUN CGO_ENABLED=0 go build -trimpath -o /out/equinox ./cmd/equinox \
 && CGO_ENABLED=0 go build -trimpath -o /out/seed ./scripts/seed

FROM python:3.13-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 TZ=America/Chicago
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY prototype/requirements.txt /app/prototype/requirements.txt
RUN pip install --no-cache-dir -r /app/prototype/requirements.txt
COPY --from=caddy:2.8 /usr/bin/caddy /usr/bin/caddy
COPY --from=build /out/equinox /app/equinox
COPY --from=build /out/seed /app/seed
COPY config /app/config
COPY fixtures/kalshi/synthetic /app/fixtures/kalshi/synthetic
COPY fixtures/polymarket/synthetic /app/fixtures/polymarket/synthetic
COPY prototype /app/prototype
RUN chmod +x /app/prototype/deploy/start.sh && mkdir -p /app/data
# Railway gives a service one domain: Caddy on $PORT sends /app to the combined app (8501) and everything
# else to the Equinox feed (8090, internal), so the feed's public URLs do not change.
EXPOSE 8080
ENTRYPOINT []
CMD ["/app/prototype/deploy/start.sh"]
