# cnpg-ui

A lightweight web UI and REST API for managing [CloudNativePG](https://cloudnative-pg.io) clusters on Kubernetes.
Designed for DBAs and developers who need to operate PostgreSQL on Kubernetes without deep K8s expertise.

## Features

- Browse, create, update, and delete CloudNativePG clusters
- Trigger and manage backups and scheduled backups
- Inspect PgBouncer poolers
- Live cluster status via Server-Sent Events (SSE)
- Stream pod logs in real time
- Session-based auth backed by a Kubernetes Secret

## Getting Started

### Prerequisites

- Go 1.23+
- Access to a Kubernetes cluster with CloudNativePG installed
- `kubectl` configured with appropriate RBAC

### Local Development

```bash
# Clone
git clone https://github.com/dbonne/cnpg-ui.git
cd cnpg-ui

# Install tools
make lint-install

# Run tests
make test

# Build
make build

# Run (reads CNPG_UI_* env vars)
export CNPG_UI_NAMESPACE=default
./bin/cnpg-ui
```

### Configuration

All settings are read from environment variables:

| Variable | Default | Description |
|---|---|---|
| `CNPG_UI_PORT` | `8080` | HTTP listen port |
| `CNPG_UI_NAMESPACE` | `default` | Kubernetes namespace to watch |
| `CNPG_UI_SESSION_TTL` | `24h` | Session expiry duration |
| `CNPG_UI_SECRET_NAME` | `cnpg-ui-credentials` | K8s Secret holding auth credentials |
| `CNPG_UI_LOG_LEVEL` | `info` | Log level: debug, info, warn, error |
| `CNPG_UI_TLS_CERT` | — | Path to TLS certificate (optional) |
| `CNPG_UI_TLS_KEY` | — | Path to TLS private key (optional) |

### Deploy with Helm

```bash
helm install cnpg-ui ./deploy/helm \
  --namespace cnpg-ui \
  --create-namespace \
  --set namespace=default
```

## Architecture

Single Go binary. chi router. HTMX for UI interactions. Server-Sent Events for live status.
See [docs/architecture.md](docs/architecture.md) for a detailed breakdown.

## License

Apache-2.0
