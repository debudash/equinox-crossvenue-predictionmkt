# Deployment

## Local (tested 2026-10-10, Windows 11, Go 1.27.2)
```
make setup && make test lint guard
go build -o equinox ./cmd/equinox
./equinox fetch
./equinox match
./equinox route --side yes          # --as-of defaults to now
./equinox serve --addr :8080        # read-only feed
./equinox rebuild                   # cards from saved raw responses, no venue calls
./equinox watch                     # background refresh; run in its own terminal next to serve
```
While watch runs, writing CLI commands are refused with a reason naming the holder (one writer at a time). Ctrl+C stops watch and releases the lock. If watch is killed hard, the lock stays: `./equinox unlock` shows the holder and `./equinox unlock --holder "<name>"` clears it.

Data lives in data/equinox.db (config/equinox.yaml `storage.sqlite_path`); raw responses are stored inside SQLite.

## Demo database (SYNTHETIC, no network)
```
go run ./scripts/seed --db data/demo.db
```
Serve it with `./equinox serve --config config/demo.yaml`. Running seed again adds a new cycle, which makes a running feed show its new-data banner.

## Docker (tested 2026-10-10)
```
docker build -t equinox .
docker run -p 8080:8080 -v "$PWD/data:/app/data" equinox                  # watch + serve (scripts/docker/start.sh)
docker run -v "$PWD/data:/app/data" equinox /app/equinox fetch            # one CLI step instead
```
The default command, scripts/docker/start.sh, clears a run lock left by a previous container (it is then the only writer), starts `watch` in the background and `serve` on `$PORT` (default 8080) as two processes; SQLite on the volume is the only handoff (A7). A container stop (SIGTERM) makes watch release the run lock. Alpine base, runs as root because hosted volumes mount root-owned. Tested locally: first discovery cycle in about 90 s, feed showing watch running, clean stop and restart.

## Railway
Deployed 2026-10-10: **https://equinox-production-6620.up.railway.app** (real data, read-only, no login).

Project successful-mercy, environment production, service `equinox`, built from the Dockerfile with `railway up`, volume `equinox-volume` mounted at `/app/data`. The container runs scripts/docker/start.sh: `watch` (discovery every 30 min, prices every 2 min) and `serve` on Railway's `$PORT`. Verified after deploy: first discovery cycle completed (457 Kalshi and 22 Polymarket US Fed cards, 10,054 pairs refused), feed returns 200 with "watch running", post pages render.

Redeploy from a clean checkout of main: `railway link --project successful-mercy --service equinox`, then `railway up --service equinox`. On Git Bash, set `MSYS_NO_PATHCONV=1` for commands that take a `/app/...` path.

## Google Cloud Run (optional): untested
SQLite on Cloud Run is ephemeral per instance, so this path is only for showing the feed over a pre-built database baked into the image or mounted from a volume. Not deployed or tested in this build.
