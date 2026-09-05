# social-relay

Private Nostr relay for [Media Centaur](https://github.com/media-centaur/media-centaur)'s Social feature. One friend group, one instance. Only members read or write; admins add members at runtime; only Media Centaur's activities — recommendations (kind 32160), watched titles (32161), tracked releases (32162) — and their deletions (kind 5) are stored.

One static binary or container image. TLS is the reverse proxy's job.

## Install

Needs a hostname pointing at the machine, ports 80 and 443, Docker with Compose.

1. Copy [`deploy/`](deploy): `compose.yml`, `Caddyfile`, `relay.toml`.
2. `Caddyfile`: set your hostname.
3. `relay.toml`: set `service_url = "wss://<hostname>"` and your npub under `admins` (**Settings → Social → Your identity**).
4. `docker compose up -d`
5. `curl -H 'Accept: application/nostr+json' https://<hostname>` returns JSON.

Members add `wss://<hostname>` under **Settings → Social → Relays**.

Bare binary: [releases](https://github.com/media-centaur/social-relay/releases), `social-relay -config relay.toml`, any proxy that forwards WebSocket upgrades. systemd and nginx in [docs/operating.md](docs/operating.md).

## Configuration

`relay.toml`. All keys required except `name`; unknown keys are errors.

| Key | Meaning |
|---|---|
| `name` | Shown in the NIP-11 document. |
| `listen` | Bind address. `0.0.0.0:2170` in the container, `127.0.0.1:2170` behind a local proxy. |
| `database` | Event database, one file. |
| `service_url` | The `wss://` address members type. Scheme, host and path must match. |
| `admins` | npubs that manage members and are members. At least one. |

## Manage

```sh
export SOCIAL_RELAY_ADMIN_KEY=nsec1...   # Settings → Social → Secret key
social-relay members -relay wss://<hostname> add <npub> [reason]
social-relay members -relay wss://<hostname> remove <npub>
social-relay members -relay wss://<hostname> list
```

Or `docker run --rm -e SOCIAL_RELAY_ADMIN_KEY ghcr.io/media-centaur/social-relay members ...`. Changes apply at once.

| Task | How |
|---|---|
| Logs | `docker compose logs relay` |
| Upgrade | `docker compose pull && docker compose up -d` |
| Back up | Stop, copy `events.db` off the `relay-data` volume, start. Members republish their own events to an empty relay. |

**Rejected** in the app: `service_url` differs from what they typed. **Connected** with *restricted*: not a member yet. More in [docs/operating.md](docs/operating.md#troubleshooting).

## Reference

- [docs/protocol.md](docs/protocol.md): wire contract with the app.
- [docs/operating.md](docs/operating.md): bare binary, systemd, nginx, troubleshooting.
- [CLAUDE.md](CLAUDE.md): contributor guide. `scripts/check` is the gate.

MIT.
