# traefik-cloudflare-gompanion

A Go reimplementation of [`docker-traefik-cloudflare-companion`](https://github.com/tiredofit/docker-traefik-cloudflare-companion) that works as a drop-in replacement for the original `cloudflare-companion` behavior.

This project mirrors the original Python script behavior and includes the fixes from upstream PRs:
- [#125](https://github.com/tiredofit/docker-traefik-cloudflare-companion/pull/125/changes): safer Traefik API response validation and JSON decode handling
- [#127](https://github.com/tiredofit/docker-traefik-cloudflare-companion/pull/127/changes): Docker event listener fixes for container/service event types

## Goals achieved

- Drop-in replacement logic for the original companion script
- Static Go binary suitable for `scratch`
- No root required
- Designed to run in read-only containers (no file writes by default)
- Multi-arch CI builds and GHCR image publishing

## Behavior compatibility

The following behavior is preserved:
- Docker container scanning (Traefik v1 and v2 label parsing)
- Docker swarm service scanning
- Optional Traefik API polling
- Cloudflare DNS upsert logic with `DRY_RUN` and `REFRESH_ENTRIES`
- `*_FILE` secret support for sensitive env vars, including `CF_TOKEN`, `CF_EMAIL`, and `DOMAINn_ZONE_ID`
- Record source priority logic (Docker mappings preferred over Traefik polling mappings)

## Differences from original tool

- Logging format is different
- `LOG_TYPE=FILE`/`BOTH` is accepted but file logging is intentionally not used so read-only mode is safe
- Implementation language is Go instead of Python

## Environment variables

All original environment variables are supported.

| Variable | Default | Description |
|---|---:|---|
| `CF_TOKEN` / `CF_TOKEN_FILE` | | Cloudflare API token (required) |
| `CF_EMAIL` / `CF_EMAIL_FILE` | | Optional Cloudflare email for global API key mode |
| `TARGET_DOMAIN` | | DNS target value for CNAME records (required) |
| `DOMAIN1`, `DOMAIN2`, ... | | Domain(s) to manage |
| `DOMAINn_ZONE_ID` / `DOMAINn_ZONE_ID_FILE` | | Cloudflare zone ID (required per domain) |
| `DOMAINn_PROXIED` | `FALSE` | Whether records are proxied |
| `DOMAINn_TTL` | `DEFAULT_TTL` | TTL for created/updated records |
| `DOMAINn_TARGET_DOMAIN` | `TARGET_DOMAIN` | Per-domain override target |
| `DOMAINn_COMMENT` | | Optional record comment |
| `DOMAINn_EXCLUDED_SUB_DOMAINS` | | Comma-separated excluded subdomains |
| `DRY_RUN` | `FALSE` | Print intended updates without applying |
| `DEFAULT_TTL` | `1` | Default Cloudflare TTL |
| `RC_TYPE` | `CNAME` | DNS record type |
| `ENABLE_DOCKER_POLL` | `TRUE` | Enable Docker inspection + event watch |
| `DOCKER_SWARM_MODE` | `FALSE` | Enable swarm service discovery |
| `TRAEFIK_VERSION` | `2` | `1` or `2` parsing logic |
| `TRAEFIK_FILTER` | | Optional value regex for filtered discovery |
| `TRAEFIK_FILTER_LABEL` | `traefik.constraint` | Label key regex used with `TRAEFIK_FILTER` |
| `ENABLE_TRAEFIK_POLL` | `FALSE` | Enable Traefik API polling |
| `TRAEFIK_POLL_URL` | | Base URL for Traefik API |
| `TRAEFIK_POLL_SECONDS` | `60` | Poll interval |
| `TRAEFIK_INCLUDED_HOSTn` | `.*` | Include host regex list |
| `TRAEFIK_EXCLUDED_HOSTn` | | Exclude host regex list |
| `REFRESH_ENTRIES` | `FALSE` | Force record update even when content matches |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `VERBOSE`, `NOTICE`, `INFO`, `WARN`, `ERROR` |
| `LOG_TYPE` | `BOTH` | Accepted for compatibility |
| `LOG_PATH` | `/logs` | Accepted for compatibility |
| `LOG_FILE` | `tcc.log` | Accepted for compatibility |

## Usage

### Docker run

```bash
docker run --rm \
  -e CF_TOKEN=... \
  -e TARGET_DOMAIN=lb.example.net \
  -e DOMAIN1=example.com \
  -e DOMAIN1_ZONE_ID=... \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/<owner>/<repo>:latest
```

### Read-only container

```bash
docker run --rm --read-only \
  -e CF_TOKEN=... \
  -e TARGET_DOMAIN=lb.example.net \
  -e DOMAIN1=example.com \
  -e DOMAIN1_ZONE_ID=... \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/<owner>/<repo>:latest
```

### Binary

```bash
CGO_ENABLED=0 go build -o cloudflare-companion ./cmd/cloudflare-companion
./cloudflare-companion
```

## Development

```bash
go test ./...
gofmt -w cmd/cloudflare-companion/*.go
```

## CI/CD

- `CI` workflow (push to `main` + pull requests): fmt, lint, tests, multi-arch binary builds, and main branch GHCR image publish
- `Release` workflow (manual): semantic bump (`major|minor|patch`), changelog generation using `git-cliff`, tag creation, GitHub release, and GHCR publish (`latest` + version)
- `Security` workflow: CodeQL analysis
- Dependabot: updates for GitHub Actions, Go modules, and Docker

## License

MIT. See [`LICENSE`](LICENSE).
