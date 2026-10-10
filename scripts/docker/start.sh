#!/bin/sh
# Container entrypoint for a hosted deployment (Railway): watch and serve run
# as two separate processes (amendment A7); SQLite on the mounted volume is the
# only handoff between them.
set -eu

PORT="${PORT:-8080}"
CONFIG="${EQUINOX_CONFIG:-config/equinox.yaml}"

# A container stopped hard can leave watch's run lock behind. At boot this
# container is the only writer, so clearing a leftover lock is safe.
holder=$(/app/equinox unlock --config "$CONFIG" 2>&1 | sed -n 's/.*held by "\(.*\)" since.*/\1/p')
if [ -n "$holder" ]; then
  /app/equinox unlock --config "$CONFIG" --holder "$holder"
fi

# watch fetches, matches and refreshes prices in the background. If it exits,
# serve keeps serving the last completed cycle (test W10).
/app/equinox watch --config "$CONFIG" &
watch_pid=$!

# Forward a container stop to watch so it releases the run lock.
trap 'kill -TERM "$watch_pid" 2>/dev/null; wait "$watch_pid" 2>/dev/null; exit 0' TERM INT

/app/equinox serve --config "$CONFIG" --addr ":$PORT" &
serve_pid=$!
wait "$serve_pid"
