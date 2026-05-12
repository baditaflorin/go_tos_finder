# 0crawl Fleet — Ecosystem Reference

This file is injected into every service repo. Read it first in any new session.

## go-common packages (github.com/baditaflorin/go-common)

| Package | Import path | Purpose |
|---------|------------|---------|
| safehttp | `github.com/baditaflorin/go-common/safehttp` | SSRF-safe HTTP client, DNS-rebind guard |
| ua | `github.com/baditaflorin/go-common/ua` | Standard User-Agent builder |

### safehttp quick-ref

```go
import (
    "github.com/baditaflorin/go-common/safehttp"
    "github.com/baditaflorin/go-common/ua"
)

client := safehttp.NewClient(
    safehttp.WithTimeout(10*time.Second),
    safehttp.WithUserAgent(ua.Build(ServiceID, Version)),
)
// Errors: safehttp.ErrBlocked, safehttp.ErrInvalidScheme, safehttp.ErrMissingHost
// NormalizeURL: adds https://, strips whitespace, validates host
u, err := safehttp.NormalizeURL(rawInput)
```

## fleet-runner (github.com/baditaflorin/go_fleet_runner)

Binary at `/usr/local/bin/fleet-runner` on builder LXC 108.

```
fleet-runner health [--insecure]             # /health on all live services
fleet-runner build-test                      # go test ./... in all workspaces
fleet-runner update-dep <mod@ver>            # bump dep across all repos
fleet-runner inject <src> <dest>             # copy a file into every repo
fleet-runner exec "<cmd>"                    # shell command in every repo
fleet-runner push "<msg>"                    # commit+push all dirty repos
fleet-runner smoke [--insecure]              # GET example_url on all services
fleet-runner new-service <name> <port> [cat] # scaffold new service
fleet-runner stats                           # audit log + token usage summary
```

All commands accept `--tokens-used N --model NAME` to log LLM token consumption.
Token totals visible in `fleet-runner stats`.

## Service conventions

- Port: from `PORT` env var; fallback to build-time constant; defined in service.yaml + compose
- Health: `GET /health` -> `{"status":"ok","service":"<id>","version":"<ver>"}`
- Version: `GET /version` -> `{"version":"<ver>"}`
- User-Agent: `ua.Build(ServiceID, Version)` -- `go_<name>/<ver> (+https://github.com/baditaflorin/go_<name>)`
- Docker image: `ghcr.io/baditaflorin/<id>:<version>` (no `v` prefix on the tag)
- service.yaml fields: `id`, `name`, `version`, `port`, `category`, `health_url`, `example_url`
- Tagging: `git tag <version>` (no `v` prefix), e.g. `1.2.3`

## Migration pattern (SSRF guard -> go-common/safehttp)

```
go get github.com/baditaflorin/go-common@v0.2.0
go mod tidy
# delete local ssrf.go / safehttp.go / fetch.go
# replace newSafeClient(ua) -> safehttp.NewClient(safehttp.WithUserAgent(ua))
# replace validateURL(u)    -> safehttp.ValidateURL(u)
# replace guardHost(host)   -> safehttp.GuardHost(ctx, host)
go build ./... && go test ./...
# bump patch version, git tag, docker buildx build --push, deploy
```

## Infrastructure access

| Target | SSH |
|--------|-----|
| Builder LXC 108 | `ssh root@0docker.com 'pct exec 108 -- bash -lc "<cmd>"'` |
| Dockerhost VM | `ssh -J root@0docker.com ubuntu_vm@10.10.10.20` |
| Webgateway | `ssh -J root@0docker.com florin@10.10.10.10` |

- Compose dirs: `/opt/services/<repo>/`, `/opt/security/<repo>/`, `/home/ubuntu_vm/pentest/<repo>/`
- Builder workspaces: `/root/workspace/go_*/`
- GHCR push: `docker buildx build --platform linux/amd64 --provenance=false -t ghcr.io/baditaflorin/<id>:<ver> --push .`
