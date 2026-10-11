# ADR-009: Host on Railway, not Google Cloud

Status: Accepted 2026-10-11. Requirements: NFR-7 ("Go, GCP preferred, local acceptable"). Decision D-22.

## Context
The brief prefers GCP. Equinox runs as two long-lived processes over one SQLite file (A7): `watch` writes on a schedule (discovery every 30 minutes, prices every 2 minutes, so prices stay inside the 10-minute age limit) and `serve` reads. That needs three things from a host: a process that never scales to zero, a persistent local disk, and SQLite file locking that works on that disk.

## Decision
- Host the Docker image on Railway: one always-on service running scripts/docker/start.sh (`watch` and `serve` as separate processes) with a persistent volume mounted at /app/data.
- Keep the Dockerfile host-neutral so the same image runs on GCP later.

## Options considered
| Option | Why rejected |
|---|---|
| Cloud Run (default) | Scales to zero and has no persistent local disk; `watch` would stop between requests and SQLite would be lost per instance |
| Cloud Run + Cloud Storage FUSE volume | SQLite locking over an object-store mount is not safe for a concurrent writer and reader |
| Cloud Run + Filestore (NFS), min instances 1 | Works, but adds a VPC connector, a Filestore instance (1 TiB minimum on the basic tier) and NFS locking caveats for one small SQLite file |
| Compute Engine VM with a persistent disk | Fits technically; adds VM, disk, firewall and TLS setup to operate for a demo |
| Replace SQLite with Cloud SQL | Changes the storage layer the build requirements fix (SQLite via modernc.org/sqlite) |
| Railway | Always-on container, persistent volume, HTTPS domain, deploy from the existing Dockerfile in minutes |

## Consequences
- The GCP preference in NFR-7 is not met for the hosted demo; local runs and the Docker image remain host-neutral.
- Moving to GCP later means a Compute Engine VM (or Cloud Run with Filestore and min instances 1) running the same image; no code change.
- docs/deployment.md keeps the Cloud Run note, marked untested, with this ADR as the reason.
