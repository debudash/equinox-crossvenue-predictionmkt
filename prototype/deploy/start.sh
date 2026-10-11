#!/bin/sh
# Combined container: the Equinox core as scripts/docker/start.sh runs it (watch + serve), the proposal app,
# the daily venue-policy refresh, and Caddy on $PORT routing /app to the app and the rest to the feed.
# SQLite on /app/data is the handoff.
set -eu
cd /app
PORT="${PORT:-8080}"
APP_PORT="${APP_PORT:-8501}"
CONFIG="${EQUINOX_CONFIG:-config/equinox.yaml}"

# A container stopped hard can leave watch's run lock behind; at boot this container is the only writer.
holder=$(/app/equinox unlock --config "$CONFIG" 2>&1 | sed -n 's/.*held by "\(.*\)" since.*/\1/p')
if [ -n "$holder" ]; then
  /app/equinox unlock --config "$CONFIG" --holder "$holder"
fi

# First boot on a fresh volume: the synthetic demo database, and the proposal state carried over from the laptop.
[ -f data/demo.db ] || /app/seed --config "$CONFIG" --db data/demo.db
[ -f data/proposals.db ] || cp prototype/deploy/seed/proposals.db data/proposals.db

/app/equinox watch --config "$CONFIG" &
watch_pid=$!
trap 'kill -TERM "$watch_pid" 2>/dev/null; wait "$watch_pid" 2>/dev/null; exit 0' TERM INT

/app/equinox serve --config "$CONFIG" --addr "127.0.0.1:8090" &

# One public port: /app -> the combined app, everything else -> the feed (prototype/deploy/Caddyfile).
PORT="$PORT" caddy run --config /app/prototype/deploy/Caddyfile --adapter caddyfile &

# Venue policies: refreshed daily at 00:15 (TZ in the image), same job as the laptop's scheduled task.
(cd prototype && python -m proto policies schedule) &

cd prototype
exec streamlit run app.py --server.port "$APP_PORT" --server.address 127.0.0.1 --server.headless true   --server.baseUrlPath app
