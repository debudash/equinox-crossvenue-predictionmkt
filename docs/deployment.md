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
Point a copy of config/equinox.yaml at data/demo.db to serve it. Running seed again adds a new cycle, which makes a running feed show its new-data banner.

## Docker (tested 2026-10-10: image builds, CLI runs)
```
docker build -t equinox .
docker run -p 8080:8080 -v "$PWD/data:/app/data" equinox serve --addr :8080
docker run -v "$PWD/data:/app/data" equinox fetch
```
Static binary on distroless (nonroot). The image includes config/ only; data comes from the mounted volume.

## Google Cloud Run (optional): untested
SQLite on Cloud Run is ephemeral per instance, so this path is only for showing the feed over a pre-built database baked into the image or mounted from a volume. Not deployed or tested in this build.
