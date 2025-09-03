# Portcrane

Programmable L4/L7 reverse proxy and load balancer written in Go.

Portcrane terminates HTTP, HTTPS, and raw TCP traffic, routes requests using declarative YAML rules, and balances load across upstream pools with health checks, middleware, and hot configuration reload.

## Quick start

```bash
go run ./cmd/portcrane -config examples/config.yaml
curl http://localhost:8080/
```

With Docker:

```bash
docker compose up --build
```

## Architecture

```
cmd/portcrane          entrypoint
internal/config        YAML schema + reload
internal/proxy         listeners and forwarding
internal/routing       host/path/header rules
internal/upstream      pools and load balancing
internal/health        active and passive checks
internal/middleware    auth, rate limit, cors
internal/tls           static certs and ACME
internal/discovery     file and DNS backends
internal/admin         REST control plane
internal/metrics       Prometheus exporters
```

Traffic enters through listeners, matches a route, passes through middleware, and is forwarded to a selected upstream from a pool.

## Configuration

See `examples/config.yaml` for a minimal working setup. Listeners, routes, pools, and middleware are all declared in YAML.

Environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORTCRANE_CONFIG` | `examples/config.yaml` | Config file path |
| `PORTCRANE_LOG_LEVEL` | `info` | slog level |
| `PORTCRANE_ADMIN_TOKEN` | empty | Admin API bearer token |
| `PORTCRANE_METRICS_ADDR` | `:9090` | Prometheus scrape address |

## Development

```bash
go test ./...
go test -race ./...
golangci-lint run
```

## License

MIT
