# Deployment

## Local
```
make setup && make test
equinox fetch
equinox match
equinox route --side yes            # --as-of defaults to now
equinox serve --addr :8080          # read-only feed
equinox rebuild                     # cards from saved raw responses, no venue calls
equinox watch                       # background refresh; run in its own terminal next to serve
```
While watch runs, writing CLI commands are refused with a reason (one writer at a time).
Data lives in data/equinox.db and data/raw (config/equinox.yaml).

## Docker
```
docker build -t equinox .
docker run -p 8080:8080 -v $PWD/data:/app/data equinox serve --addr :8080
```

## Google Cloud Run (optional)
SQLite on Cloud Run is ephemeral per instance, so this path is for showing the feed over a pre-built database baked into the image or mounted from a volume. Steps are recorded by T-30 as tested or untested.
