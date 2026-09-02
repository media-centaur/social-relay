# Operating a relay

One friend group runs one relay. It is a single static binary, also shipped as a container image, configured by one TOML file that names the relay's admins. Admins add and remove members while the relay runs. Members paste the relay's address into Media Centaur under **Settings → Social → Relays**.

The relay speaks plain HTTP and WebSocket. TLS is the job of a reverse proxy in front of it; the Compose setup below uses Caddy, which obtains certificates itself.

## What you need

- A machine reachable by every member, with a hostname pointing at it.
- Ports 80 and 443 open to the internet, or an existing reverse proxy that forwards WebSocket upgrades.
- Docker with Compose, or nothing beyond the binary.
- Your own npub, to be the first admin. In Media Centaur it is under **Settings → Social → Your identity**, with a **Copy** button. Members' npubs come from the same place on their installs.

## Run with Docker Compose

1. Copy the three files from [`deploy/`](../deploy) into a directory of their own: `compose.yml`, `Caddyfile`, `relay.toml`.
2. In `Caddyfile`, replace `relay.example.com` with your hostname.
3. In `relay.toml`, set `service_url` to `wss://` followed by the same hostname, and put your npub under `admins`.
4. Start it:

   ```sh
   docker compose up -d
   ```

5. Confirm it answers. The relay information document is served on the same address:

   ```sh
   curl -H 'Accept: application/nostr+json' https://relay.example.com
   ```

   Expected: a JSON document with `"supported_nips":[1,11,42]` and the version.

6. Add each member (see **Members** below), then tell them to add `wss://relay.example.com` under **Settings → Social → Relays**. Their row reads **Connected** once the relay accepts their key.

The event database lives on the `relay-data` volume. The image runs as an unprivileged user (uid 65532); a fresh named volume is created writable for it. If you bind-mount a host directory instead, make it owned by that uid.

## Run the binary

1. Download `social-relay_linux_amd64` or `social-relay_linux_arm64` and `SHA256SUMS` from the [releases](https://github.com/media-centaur/social-relay/releases), and verify:

   ```sh
   sha256sum -c SHA256SUMS --ignore-missing
   ```

2. Install it and create a service user with a data directory:

   ```sh
   sudo install -m 755 social-relay_linux_amd64 /usr/local/bin/social-relay
   sudo useradd --system --home /var/lib/social-relay --create-home social-relay
   sudo mkdir -p /etc/social-relay
   ```

3. Write `/etc/social-relay/relay.toml` from [`relay.example.toml`](../relay.example.toml). Keep `listen = "127.0.0.1:2170"` so only the proxy on the same machine can reach it, and set `database = "/var/lib/social-relay/events.db"`.

4. Run it under systemd, `/etc/systemd/system/social-relay.service`:

   ```ini
   [Unit]
   Description=social-relay
   After=network-online.target
   Wants=network-online.target

   [Service]
   User=social-relay
   ExecStart=/usr/local/bin/social-relay -config /etc/social-relay/relay.toml
   Restart=on-failure

   [Install]
   WantedBy=multi-user.target
   ```

   ```sh
   sudo systemctl enable --now social-relay
   ```

5. Put the proxy in front. The proxy must forward WebSocket upgrades. Caddy does so by default:

   ```
   relay.example.com {
   	reverse_proxy 127.0.0.1:2170
   }
   ```

   nginx needs the upgrade headers spelled out:

   ```nginx
   location / {
       proxy_pass http://127.0.0.1:2170;
       proxy_http_version 1.1;
       proxy_set_header Upgrade $http_upgrade;
       proxy_set_header Connection "upgrade";
       proxy_set_header Host $host;
       proxy_read_timeout 1h;
   }
   ```

   The idle timeout matters: the relay pings every 30 seconds, and a proxy that closes idle connections sooner than that disconnects members.

## Configuration reference

All keys live in one TOML file. Unknown keys are an error, so a typo does not go unnoticed.

| Key | Required | Meaning |
|---|---|---|
| `name` | No | Shown in the relay information document. |
| `listen` | Yes | Address and port the relay binds, plain HTTP. `127.0.0.1:2170` for a proxy on the same machine; `0.0.0.0:2170` inside a container. |
| `database` | Yes | Path of the event database, one file. Created on first start. |
| `service_url` | Yes | The address members paste into Media Centaur, `ws://` or `wss://`. Clients name it when they authenticate, so scheme, host and path must match exactly what members type. Letter case and a trailing slash do not matter. With a path, the relay answers only on that path. |
| `admins` | Yes, at least one | npubs that may add and remove members while the relay runs, and are members themselves. A group that never changes can list everyone here. |

Changes to this file take effect on restart. Membership changes do not go through this file.

## Members

Membership is the whole of access control. A member reads everything on the relay and may publish; anyone else can connect and authenticate, but every request and every publish is refused. Members are the admins from the config file plus every key an admin has allowed. Allowed keys live in the database and survive restarts.

Admins manage members with the same binary, from any machine that can reach the relay. The admin's secret key is read from `SOCIAL_RELAY_ADMIN_KEY`, as `nsec1…` or hex. In Media Centaur it is under **Settings → Social → Secret key → Show secret key**.

```sh
export SOCIAL_RELAY_ADMIN_KEY=nsec1...
social-relay members -relay wss://relay.example.com add npub1... "Alice"
social-relay members -relay wss://relay.example.com remove npub1... "left the group"
social-relay members -relay wss://relay.example.com list
```

Without a local binary, the image runs the same subcommand from any machine: `docker run --rm -e SOCIAL_RELAY_ADMIN_KEY ghcr.io/media-centaur/social-relay members -relay wss://relay.example.com ...`, with the variable set in the calling shell.

Changes apply at once. A newly allowed member whose Media Centaur is already connected starts reading on its next request; nobody reconnects. A removed member's open connection stops receiving events immediately and their next request is refused. Their earlier recommendations stay in the database until they are replaced; nobody can delete them.

The relay stores one recommendation per member per title. Recommending the same title again replaces the earlier event, so the database grows with members × titles and nothing else.

## Logs

The relay logs to standard error: `docker compose logs relay`, or `journalctl -u social-relay`.

| Line | Meaning |
|---|---|
| `social-relay v1.2.3 listening on …, service URL …, N admins` | Started with that config. |
| `authenticated npub1… (admin)` / `(member)` | A member connected and proved their key. |
| `authenticated npub1… (not a member)` | Someone with a valid key that is not a member. Their requests will be refused. `social-relay members add` if they belong. |
| `allowed npub1… (reason)` / `unallowed npub1… (reason)` | An admin changed membership. |

A member whose authentication itself fails, for example because their `service_url` does not match, is not logged; the reason is sent to their Media Centaur and shown on the relay row there.

## Backup

Stop the relay, copy the database file, start it again:

```sh
docker compose stop relay && docker run --rm -v relay-data:/data -v "$PWD":/backup alpine cp /data/events.db /backup/ && docker compose start relay
```

Losing the database is not fatal. Every member's Media Centaur keeps its own recommendations and republishes any the relay lacks the next time it connects, so a fresh database refills from the members within a minute of them reconnecting.

## Upgrading

Compose: `docker compose pull && docker compose up -d`. Binary: replace the file and restart the service. The database format is forward-compatible within a major version.

## Troubleshooting

What the member sees on their relay row, and what it means on this side.

| Relay row shows | Cause | Fix |
|---|---|---|
| **Rejected** | Their authentication was refused. Almost always `service_url` differs from the address they typed: `ws://` against `wss://`, a different host, or a path on one side only. | Make `service_url` and the address members type identical. Then the member removes and re-adds the relay. |
| **Connected** with *restricted: this key is not a member of this relay* | Their key authenticated but is not a member. The log shows `authenticated npub1… (not a member)`. | `social-relay members add <npub>`. No restart. |
| **Not connected** | The proxy, DNS or TLS is not in place, or the proxy does not forward WebSocket upgrades. | Run the `curl` check from the Compose steps. A JSON document means the relay and proxy are fine and the problem is the WebSocket upgrade; anything else is DNS, TLS or the proxy itself. |
| **Connected**, but a friend's recommendations never arrive | The friend is not a member, or the friend has not added this relay. | Both must be members and both must have added the relay. `social-relay members list` shows who is. |
