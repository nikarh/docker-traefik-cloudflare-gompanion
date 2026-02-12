# traefik-cloudflare-gompanion

[![CI](https://github.com/nikarh/docker-traefik-cloudflare-gompanion/actions/workflows/ci.yml/badge.svg)](https://github.com/nikarh/docker-traefik-cloudflare-gompanion/actions/workflows/ci.yml)
[![Security](https://github.com/nikarh/docker-traefik-cloudflare-gompanion/actions/workflows/security.yml/badge.svg)](https://github.com/nikarh/docker-traefik-cloudflare-gompanion/actions/workflows/security.yml)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

A Go reimplementation of [`docker-traefik-cloudflare-companion`](https://github.com/tiredofit/docker-traefik-cloudflare-companion) intended to be a drop-in replacement for the original `cloudflare-companion` script.

It preserves original behavior and includes upstream fixes:
- [PR #125](https://github.com/tiredofit/docker-traefik-cloudflare-companion/pull/125/changes): robust Traefik API response validation and JSON decode handling
- [PR #127](https://github.com/tiredofit/docker-traefik-cloudflare-companion/pull/127/changes): corrected Docker event listener handling for container and service events

## Why this rewrite exists

- Go static binary: no Python runtime, lower startup overhead, simpler supply chain.
- `scratch` image: minimal attack surface and very small footprint (typically sub-10MB compressed image layers).
- Non-root by default: container runs as `uid:gid 65532:65532`.
- Read-only-friendly runtime: no required filesystem writes, suitable for hardened container setups.

## Container tags

Published to `ghcr.io/nikarh/docker-traefik-cloudflare-gompanion`.

Main branch publish tags:
- `:main` points to the latest successful build from `main`.
- `:latest` also points to the latest successful build from `main`.
- `:sha-<commit_sha>` is an immutable CI build tag for a specific commit.

Release publish tags:
- `:<ref tag>` for the release git tag (example: `:v1.4.2`).
- `:<major.minor.patch>` for exact semver (example: `:1.4.2`).
- `:<major.minor>` moving minor line (example: `:1.4`).
- `:<major>` moving major line (example: `:1`).
- `:latest` points to the newest release publish.

## Security and Docker socket guidance

The container runs as `65532:65532`, so direct Docker socket usage requires socket permissions that allow this user.

Directly mounting `/var/run/docker.sock` gives effectively root-equivalent control of the host Docker daemon. That is a major security risk and strongly discouraged.

Recommended approach: run through [11notes/docker-socket-proxy](https://github.com/11notes/docker-socket-proxy) and only expose required Docker API capabilities.

## Behavior compatibility

The following behavior from the original companion is supported:
- Docker container scanning for Traefik v1 and v2 labels
- Docker swarm service scanning
- Optional Traefik API polling
- Cloudflare DNS create/update behavior with `DRY_RUN` and `REFRESH_ENTRIES`
- `*_FILE` secret support for sensitive values (`CF_TOKEN`, `CF_EMAIL`, `DOMAINn_ZONE_ID`)
- Mapping source precedence (Docker event/discovery mappings preferred over Traefik poll mappings)

## Differences from original

- Logging format is different.
- `LOG_TYPE=FILE` and `LOG_TYPE=BOTH` are accepted for compatibility, but this implementation does not write log files.
- Implementation is Go, not Python.

## Environment variables

All environment variables from the original tool are supported.

| Variable | Default | Description |
|---|---:|---|
| `CF_TOKEN` / `CF_TOKEN_FILE` | | Cloudflare API token (required) |
| `CF_EMAIL` / `CF_EMAIL_FILE` | | Optional Cloudflare email for global key mode |
| `TARGET_DOMAIN` | | DNS target value for records (required) |
| `DOMAIN1`, `DOMAIN2`, ... | | Domain(s) to manage |
| `DOMAINn_ZONE_ID` / `DOMAINn_ZONE_ID_FILE` | | Cloudflare zone ID (required per domain) |
| `DOMAINn_PROXIED` | `FALSE` | Whether records are proxied |
| `DOMAINn_TTL` | `DEFAULT_TTL` | TTL for records |
| `DOMAINn_TARGET_DOMAIN` | `TARGET_DOMAIN` | Per-domain target override |
| `DOMAINn_COMMENT` | | Optional record comment |
| `DOMAINn_EXCLUDED_SUB_DOMAINS` | | Comma-separated excluded subdomains |
| `DRY_RUN` | `FALSE` | Print intended updates without applying |
| `DEFAULT_TTL` | `1` | Default Cloudflare TTL |
| `RC_TYPE` | `CNAME` | DNS record type |
| `ENABLE_DOCKER_POLL` | `TRUE` | Enable Docker inspection and events |
| `DOCKER_SWARM_MODE` | `FALSE` | Enable swarm service discovery |
| `TRAEFIK_VERSION` | `2` | `1` or `2` rule parsing logic |
| `TRAEFIK_FILTER` | | Optional value regex for filtered discovery |
| `TRAEFIK_FILTER_LABEL` | `traefik.constraint` | Label key regex used with `TRAEFIK_FILTER` |
| `ENABLE_TRAEFIK_POLL` | `FALSE` | Enable Traefik API polling |
| `TRAEFIK_POLL_URL` | | Base URL for Traefik API |
| `TRAEFIK_POLL_SECONDS` | `60` | Poll interval |
| `TRAEFIK_INCLUDED_HOSTn` | `.*` | Include host regex list |
| `TRAEFIK_EXCLUDED_HOSTn` | | Exclude host regex list |
| `REFRESH_ENTRIES` | `FALSE` | Force update when content already matches |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `VERBOSE`, `NOTICE`, `INFO`, `WARN`, `ERROR` |
| `LOG_TYPE` | `BOTH` | Accepted for compatibility |
| `LOG_PATH` | `/logs` | Accepted for compatibility |
| `LOG_FILE` | `tcc.log` | Accepted for compatibility |

## Usage

### Recommended with Docker Socket Proxy

```bash
docker run --rm \
  -e CF_TOKEN=... \
  -e TARGET_DOMAIN=lb.example.net \
  -e DOMAIN1=example.com \
  -e DOMAIN1_ZONE_ID=... \
  -e DOCKER_HOST=tcp://docker-socket-proxy:2375 \
  ghcr.io/nikarh/docker-traefik-cloudflare-gompanion:main
```

### Direct Docker socket mount (discouraged)

```bash
docker run --rm --read-only \
  -e CF_TOKEN=... \
  -e TARGET_DOMAIN=lb.example.net \
  -e DOMAIN1=example.com \
  -e DOMAIN1_ZONE_ID=... \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/nikarh/docker-traefik-cloudflare-gompanion:main
```

### Build local binary

```bash
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o cloudflare-companion ./cmd/cloudflare-companion
./cloudflare-companion
```

## CI and release

- `CI` workflow (push to `main`, pull requests): fmt, tests, lint, multi-arch binary build, and GHCR image publish.
- `Release` workflow (manual): bump semantic version (`major`, `minor`, `patch`), generate changelog with `git-cliff`, create GitHub release, and publish semver Docker tags.
- `Security` workflow: CodeQL analysis.
- Dependabot: automated updates for Actions, Go modules, and Docker definitions.

## License

MIT. See [`LICENSE`](LICENSE).
