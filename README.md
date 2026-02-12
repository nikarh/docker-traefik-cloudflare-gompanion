# traefik-cloudflare-gompanion

[![CI](https://github.com/nikarh/docker-traefik-cloudflare-gompanion/actions/workflows/ci.yml/badge.svg)](https://github.com/nikarh/docker-traefik-cloudflare-gompanion/actions/workflows/ci.yml)
[![Security](https://github.com/nikarh/docker-traefik-cloudflare-gompanion/actions/workflows/security.yml/badge.svg)](https://github.com/nikarh/docker-traefik-cloudflare-gompanion/actions/workflows/security.yml)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

A Go rewrite of [tiredofit/docker-traefik-cloudflare-companion](https://github.com/tiredofit/docker-traefik-cloudflare-companion) with drop-in environment compatibility.

## Why this project

This image keeps the behavior of the original tool, but focuses on hardened runtime defaults:
- Static Go binary instead of Python runtime.
- `scratch` final image with very small footprint (typically under 10 MB).
- Runs as non-root user `65532:65532` by default.
- Works in read-only containers because it does not require writing to disk.

These choices reduce attack surface, reduce dependency chain size, and make production hardening easier.

## Compatibility scope

- Supports Traefik v1 and v2 label discovery from Docker containers.
- Supports Docker Swarm service discovery.
- Supports Traefik API polling mode.
- Supports the original Cloudflare DNS sync behavior with `DRY_RUN` and `REFRESH_ENTRIES`.
- Supports all original environment variables.

## Container tags

Published image: `ghcr.io/nikarh/docker-traefik-cloudflare-gompanion`

Main branch tags:
- `:main` latest build from `main`.
- `:latest` latest build from `main`.
- `:sha-<commit>` immutable commit build.

Release tags:
- `:v<major>.<minor>.<patch>` release git tag (for example `:v1.2.3`).
- `:<major>.<minor>.<patch>` exact semver image tag (`:1.2.3`).
- `:<major>.<minor>` moving minor line (`:1.2`).
- `:<major>` moving major line (`:1`).
- `:latest` points to the latest released version.

## Runtime user and Docker socket access

The container runs as `uid:gid 65532:65532`.

If you mount the host Docker socket directly, this user must have permission to access that socket. On many hosts this requires custom group mapping or running as a different uid/gid.

Mounting `/var/run/docker.sock` directly is a major security risk and strongly discouraged. It effectively grants privileged host control to the container.

Recommended pattern is [11notes/docker-socket-proxy](https://github.com/11notes/docker-socket-proxy), exposing only the API surface you actually need.

## Docker Compose

Reference example: [`examples/compose.yml`](examples/compose.yml)

This example runs `traefik-cloudflare-gompanion` behind socket-proxy, in read-only mode, and uses Docker secrets for Cloudflare credentials.

## Secrets and environment variable behavior

This project supports all original env vars and secret patterns.

### Secret resolution order

For any secret-enabled variable (for example `CF_TOKEN`, `CF_EMAIL`, `DOMAIN1_ZONE_ID`), value resolution is:

1. `<VAR>_FILE` path, then `<var>_FILE` path.
2. Docker default secret paths:
   - `/run/secrets/<VAR>`
   - `/run/secrets/<var>`
3. Environment variable values:
   - `<VAR>`
   - `<var>`

If `<VAR>_FILE` contains a plain name instead of absolute path, `/run/secrets/<name>` is also attempted.

This means you can use either explicit `_FILE` env vars or plain Docker secret names in `/run/secrets`.

## Environment variables

All original environment variables are supported.

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

## Quick run example

```bash
docker run --rm --read-only \
  -e CF_TOKEN=... \
  -e TARGET_DOMAIN=lb.example.net \
  -e DOMAIN1=example.net \
  -e DOMAIN1_ZONE_ID=... \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/nikarh/docker-traefik-cloudflare-gompanion:main
```

## License

MIT. See [`LICENSE`](LICENSE).
