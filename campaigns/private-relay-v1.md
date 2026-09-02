---
status: planning
started: 2026-09-02
last_updated: 2026-09-02
---
# Private relay v1

## Goal

A friend group runs one container with a list of member keys and gets a private recommendation feed that Media Centaur speaks to with no app-side change. The app's friend network shipped with **no default relay** and "private first" as its posture (app spec decision 4), so until this relay exists the feature has no first deployment target.

## Glossary

- **Relay** — a Nostr WebSocket server that stores and forwards events (NIP-01).
- **Member** — a public key on the relay's allowlist. Membership grants both read and write.
- **Allowlist** — the operator-maintained set of member keys. The only access-control primitive in v1.
- **Operator** — the person running the relay instance for their friend group.
- **Recommendation event** — the app's kind `32160` addressable event, `d` tag `tmdb:<media_type>:<tmdb_id>`, content a JSON title snapshot plus optional note. Defined by the app; the relay stores it and never interprets it.
- **Challenge** — the NIP-42 `AUTH` string the relay sends when a socket opens.
- **Service URL** — the public `wss://` URL members paste into the app. The `relay` tag of the client's `AUTH` answer carries it, and the relay checks it.

## Status

Planning. No code. Repository seeded 2026-09-02 with this file and `CLAUDE.md` from the app-side design conversation.

## Client contract (facts the relay must honour)

From `../media-centaur-app/lib/media_centaur/nostr/connection.ex` and `docs/friends.md` as of 2026-09-02.

- The client answers any `AUTH` challenge immediately with a kind `22242` event whose `relay` tag is the URL exactly as the user configured it. It does **not** re-authenticate in response to a `CLOSED` or `OK` carrying an `auth-required:` prefix. So the relay must challenge on connect.
- After a successful `AUTH` (an `OK` for the auth event with `true`) the client re-issues every subscription it holds. So refusing a `REQ` that arrived before auth is harmless.
- On `OK … false` for the auth event the client enters `auth_failed`, which the app surfaces as the incident *Relay rejected this identity*. This is the only path that surfaces as an auth failure in the app.
- A `CLOSED` on any subscription is folded into the relay row's `last_error` and shown on the Friends tab. The connection state stays connected.
- Subscriptions: `feed` (authors = friends plus self, kind 32160) and `own:<url>` (authors = self). On `EOSE` for `own:<url>` the client publishes every stored own event the relay did not return. Correct `EOSE` and correct addressable semantics therefore matter: a relay that fails to return an event it holds gets it published again, and one that appends instead of replacing returns duplicates.
- Publishes are casts; the client reads the relay's verdict from `OK`.
- Reconnect backoff is 1 s doubling to 60 s. Mint's connect timeout is capped at 5 s.

## Decisions made

* `2026-09-02` — **khatru, not a turnkey relay.** The requirement is NIP-42 gating of *reads and writes* by allowlist. strfry gates writes only (write-policy plugin) and nostr-rs-relay's `pubkey_whitelist` covers event publishing only; both would need a fork. khatru makes read gating a `RejectFilter` hook. Building from scratch was rejected: replaceable-event semantics, subscription bookkeeping and the NIP-42 state machine are the parts worth not rewriting. (Design conversation in the app repo, 2026-09-02.)
* `2026-09-02` — **Module path `fiatjaf.com/nostr/khatru`, exact pseudo-version pinned.** The GitHub repo is archived; the archived `github.com/fiatjaf/khatru` is the fallback if the new path proves unstable.
* `2026-09-02` — **SQLite eventstore.** One file, no service dependency. Volume is bounded by members × titles.
* `2026-09-02` — **Allowlist in a TOML config file**, one list of `npub` strings. Restart to apply in v1.
* `2026-09-02` — **Challenge on connect** (`RequestAuth` from `OnConnect`), because the client answers challenges proactively and never reacts to `auth-required:` rejections.
* `2026-09-02` — **Kinds accepted: 32160 only.** The auth kind 22242 is handled by the framework. Anything else is rejected with a `blocked:` reason.
* `2026-09-02` — **TLS terminates at the operator's reverse proxy.** The relay listens on plain HTTP on loopback or a container network. Documented, not built.
* `2026-09-02` — **Built in this repo by its own Claude Code instance.** The app instance seeds the docs and owns app-side changes; this instance owns the Go code. The app's skills, memory and `CLAUDE.md` are Elixir-specific and would mis-route work here.

## Next steps

1. `go mod init github.com/media-centaur/social-relay`; pin khatru and the SQLite eventstore; a relay that starts, serves a NIP-11 document naming itself and its supported NIPs (1, 11, 42), and stores events. `scripts/check` and CI (check on push) land in the same step.
2. Allowlist config and NIP-42 gating. Tests through a real client on loopback: unauthenticated `REQ` gets `CLOSED` with `auth-required:`; a non-member authenticates then gets `CLOSED` / `OK false` with `restricted:`; a member round-trips an event; a second event with the same author and `d` replaces the first and a `REQ` returns only the newer one; `EOSE` follows stored matches.
3. Kind restriction. Kind 1 rejected with `blocked:`; kind 32160 accepted.
4. Service URL check. Confirm what khatru compares the `relay` tag against and make the public URL an explicit config value. Test with and without a trailing slash and with `ws://` vs `wss://`.
5. Container image (scratch, non-root, config and database on a volume), a compose example with a reverse proxy, `docs/operating.md`.
6. End to end against the dev app on `:2160`, then write `docs/protocol.md` and cross-check it against `docs/friends.md` line by line.
7. Tag a release; wiki page *Hosting a private relay* in `../media-centaur.wiki`.

## Open questions

1. **How a non-member's rejection surfaces in the app.** khatru validates `AUTH` by signature, so a non-member's auth succeeds and the rejection arrives on the first `REQ` as `CLOSED restricted:`. The app then shows *connected* with a `last_error`, not the *Relay rejected this identity* incident. Verify khatru's behaviour first; if it cannot refuse the auth event itself, the fix is app-side (treat `restricted:` on the `feed` subscription as an auth failure). Cross-repo.
2. **Allowlist changes without restart.** Restart is acceptable for a friend group. Revisit only if operators ask.
3. **Deletion and retention.** The app never sends kind 5. Reject it with `blocked:` for now. Retention is unnecessary while storage is bounded by addressable events.
4. **Directed recommendations later** (NIP-17 gift wraps, kind 1059) widen the kind allowlist and add recipient-scoped read rules. Not v1; the kind list must be a single config point so this is a one-line change.

## Cross-repo

Items that need an app-side change. Move each to the app campaign `../media-centaur-app/campaigns/friends-recommendations.md` when it becomes concrete.

* Open question 1 above, if khatru cannot refuse `AUTH` for non-members.

## Completion criteria

* `docker run` with a mounted config file yields a relay that challenges on connect, refuses `REQ` and `EVENT` from non-members, stores kind 32160 with replace semantics, and rejects every other kind.
* A recommendation sent from the dev app is received by a second client (a second app instance or a scripted go-nostr client) through this relay, with no app-side change.
* A tagged release with a container image on GHCR, and the wiki page.
* `docs/protocol.md` exists and agrees with `docs/friends.md`.

## Pointers

* App contributor guide: `../media-centaur-app/docs/friends.md`.
* App design spec: `../media-centaur-app/docs/superpowers/specs/2026-09-02-friends-recommendations-design.md`.
* App campaign: `../media-centaur-app/campaigns/friends-recommendations.md`.
* App test relay (the subset the client uses, in Elixir): `../media-centaur-app/test/support/nostr/fake_relay.ex`.
* khatru: https://pkg.go.dev/fiatjaf.com/nostr/khatru (live), https://github.com/fiatjaf/khatru (archived).
* NIPs: 01 (events, filters, `OK`/`CLOSED` prefixes), 11 (relay information document), 42 (client authentication).
