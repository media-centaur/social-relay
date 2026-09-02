# social-relay

The private relay for [Media Centaur](https://github.com/media-centaur/media-centaur)'s Social feature. One friend group runs one instance. Only members can read or write, admins add members while it runs, and it stores only Media Centaur's recommendation events.

Single static binary, also as a container image. Plain HTTP and WebSocket; TLS is your reverse proxy's job.

## Install

Needs a hostname pointing at the machine, ports 80 and 443 open, and Docker with Compose.

1. Copy [`deploy/`](deploy) somewhere: `compose.yml`, `Caddyfile`, `relay.toml`.
2. `Caddyfile`: replace `relay.example.com` with your hostname.
3. `relay.toml`: set `service_url = "wss://<your hostname>"` and put your npub under `admins`. It is under **Settings → Social → Your identity**.
4. `docker compose up -d`
5. Check: `curl -H 'Accept: application/nostr+json' https://<your hostname>` returns a JSON document.

Add members (below), then they add `wss://<your hostname>` under **Settings → Social → Relays**. Their row reads **Connected**.

Without Docker: binaries and checksums are on the [releases page](https://github.com/media-centaur/social-relay/releases); run `social-relay -config relay.toml` behind any proxy that forwards WebSocket upgrades. See [docs/operating.md](docs/operating.md) for systemd and nginx.

## Configuration

`relay.toml`, all keys required except `name`. Unknown keys are errors.

| Key | Meaning |
|---|---|
| `name` | Shown in the relay information document. |
| `listen` | Bind address. `0.0.0.0:2170` in the container, `127.0.0.1:2170` behind a local proxy. |
| `database` | The event database, one file. |
| `service_url` | The `ws://` or `wss://` address members type. Scheme, host and path must match exactly. |
| `admins` | npubs that manage members and are members themselves. At least one. |

## Manage

| Task | How |
|---|---|
| Add or remove a member | `SOCIAL_RELAY_ADMIN_KEY=nsec1... social-relay members -relay wss://<host> add <npub> [reason]`, or `remove`, or `list`. Applies at once, no restart. The nsec is under **Settings → Social → Secret key**. |
| See who is connecting | `docker compose logs relay`. Each authentication logs the npub and whether it is a member. |
| Upgrade | `docker compose pull && docker compose up -d` |
| Back up | Stop the relay, copy `events.db` off the `relay-data` volume, start it. Members republish their own recommendations to an empty relay, so the database is recoverable without one. |

A member's row reading **Rejected** means `service_url` differs from what they typed. **Connected** with *restricted: this key is not a member* means no admin has added them yet. Full table in [docs/operating.md](docs/operating.md#troubleshooting).

## Reference

- [docs/protocol.md](docs/protocol.md): what the relay accepts and answers, checked against the app.
- [docs/operating.md](docs/operating.md): bare binary, systemd, nginx, full troubleshooting.
- [CLAUDE.md](CLAUDE.md): contributor guide. `scripts/check` is the gate.

MIT licensed.
