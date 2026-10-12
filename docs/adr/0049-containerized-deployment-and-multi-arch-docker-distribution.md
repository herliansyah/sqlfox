# 0049: Containerized Deployment & Multi-Arch Docker Distribution

## Status
Accepted

## Context
SqlFox's primary deployment model has been standalone native binaries for Linux, macOS, and Windows. However, operators managing self-hosted infrastructure, homelabs, VPS servers, and internal developer platforms frequently demand Docker container distribution:
1. **Zero-Friction Self-Hosting**: Running database management dashboards alongside microservices or containerized databases without manual binary installation, systemd daemon management, or OS user provisioning.
2. **Container Network Colocation**: In `docker-compose.yml` environments, containerized tools can reach target database services (`mysql:3306`, `postgres:5432`) directly via Docker bridge DNS without exposing database ports to the host interface.
3. **Reproducible Ephemeral & Persistent States**: Vault configuration (`config.db`), auto-generated `master.key`, and pre-DDL backup dumps require deterministic volume isolation.

## Decisions

### 1. Multi-Stage Alpine Dockerfile
We implement an ultra-lightweight multi-stage `Dockerfile`:
- **Builder Stage**: `golang:alpine` builds the pure-Go binary with `-trimpath` and stripped symbols (`-ldflags="-s -w"`), preserving the CGO-Free Invariant (`CGO_ENABLED=0`).
- **Runtime Stage**: `alpine:3.20` equipped only with `ca-certificates` (for TLS/SSL database connections and BYOK AI HTTPS calls) and `tzdata` (for correct timestamp timezone parsing).
- **Resulting Image Size**: **~20 MB total image size**, maintaining an idle RAM footprint of **<25 MB**.

### 2. Non-Root Security Model
The container runs under a dedicated unprivileged user (`sqlfox:sqlfox`, UID/GID 10001) rather than root. The working directory is `/home/sqlfox`, and write permissions are explicitly granted to the `/data` volume mount.

### 3. Dedicated Persistent Volume Mount (`/data`)
Container persistence is consolidated into a single dedicated volume (`/data`):
- Default command executes: `ENTRYPOINT ["/usr/local/bin/sqlfox"]` with `CMD ["-host", "0.0.0.0", "-no-browser", "-db", "/data/config.db"]`.
- `config.db`, `master.key`, and `backups/` all reside predictably within `/data`, enabling straightforward host bind mounts (`-v ./sqlfox-data:/data`) or named volumes (`-v sqlfox_data:/data`).
- Environment variable `SQLFOX_PASS` can optionally supply a static Master PIN/password, bypassing random PIN generation.

### 4. Host-Level Database Connectivity (`Host Gateway`)
To enable SqlFox inside a container to reach databases running natively on the host machine:
- `docker-compose.yml` pre-configures `extra_hosts: ["host.docker.internal:host-gateway"]`.
- Operators connecting to host-level MySQL/PostgreSQL specify host `host.docker.internal` rather than `localhost`/`127.0.0.1`.

### 5. Multi-Architecture CI/CD Automation (GHCR)
`.github/workflows/release.yml` integrates QEMU and Docker Buildx to cross-compile and publish multi-arch container images (`linux/amd64` and `linux/arm64`) to GitHub Container Registry (`ghcr.io/herliansyah/sqlfox`) on every release tag.

## Consequences
- Operators can deploy SqlFox in under 5 seconds with a single `docker run` or `docker compose up -d` command.
- Zero impact on native desktop single-binary builds; containerization is an additive distribution channel.
- Fully adheres to Ponytail principles: minimal layer footprints, no bloated runtimes, and zero non-standard dependencies.
