---
status: in-progress
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

In progress. Steps 1 to 4 done 2026-09-02 and pushed to `github.com/media-centaur/social-relay`: module pinned, bbolt store, TOML config (`name`, `listen`, `database`, `service_url`, `members`), NIP-11 advertising 1/11/42, replace semantics, challenge on connect, membership gating of reads and writes, kind 32160 only, service URL check. Step 5 done 2026-09-02: scratch image (uid 65532, 4.5 MB, built and smoke-run locally), `deploy/` compose with Caddy, release workflow (binaries plus GHCR image on `v*` tags), `docs/operating.md`, README, auth logging. Step 6 done 2026-09-02: dev app on `:2160` added `ws://127.0.0.1:2173`, relay log showed its npub authenticating as a member, the Social row read **Connected**, the Status drill-in read *Connected to 1 of 1 relays · 1 sent*, a recommendation sent from a library title reached a scripted second member through the relay (kind 32160, `d` = `tmdb:movie:…`). No app-side change. `docs/protocol.md` written and cross-checked against `docs/social.md`. The test relay was removed from the dev app afterwards; the recommendation it created remains in the dev database. Step 7 (release, wiki) is next.

Layout: `cmd/social-relay` (flag `-config`, graceful shutdown), `internal/relay` (`config.go`, `relay.go` wiring, `membership.go`, `kinds.go`), tests in `internal/relay/*_test.go` through `httptest.NewServer` and a raw WebSocket client (`client_test.go`) that answers AUTH and waits for its `OK` the way the app does.

Rejection reasons, fixed for `docs/protocol.md`: unauthenticated `REQ` → `CLOSED auth-required: authenticate to read from this relay`; unauthenticated `EVENT` → `OK false auth-required: authenticate to write to this relay`; authenticated non-member → `restricted: this key is not a member of this relay` on both; member publishing an outsider's event → `restricted: the event author is not a member of this relay`; other kinds → `blocked: kind N is not stored by this relay`.

Reconciled 2026-09-02 against the live khatru module before the first line of code. Three seeded assumptions changed; see **Decisions made** for the module layout, the storage backend (ADR-001) and the AUTH refusal answer.

## Client contract (facts the relay must honour)

From `../media-centaur-app/lib/media_centaur/nostr/connection.ex` and `docs/social.md` as of 2026-09-02.

- The client answers any `AUTH` challenge immediately with a kind `22242` event whose `relay` tag is the URL exactly as the user configured it. It does **not** re-authenticate in response to a `CLOSED` or `OK` carrying an `auth-required:` prefix. So the relay must challenge on connect.
- After a successful `AUTH` (an `OK` for the auth event with `true`) the client re-issues every subscription it holds. So refusing a `REQ` that arrived before auth is harmless.
- On `OK … false` for the auth event the client enters `auth_failed`, which the app surfaces as the incident *Relay rejected this identity*. This is the only path that surfaces as an auth failure in the app.
- A `CLOSED` on any subscription is folded into the relay row's `last_error` and shown on the Social tab. The connection state stays connected.
- Subscriptions: `feed` (authors = friends plus self, kind 32160) and `own:<url>` (authors = self). On `EOSE` for `own:<url>` the client publishes every stored own event the relay did not return. Correct `EOSE` and correct addressable semantics therefore matter: a relay that fails to return an event it holds gets it published again, and one that appends instead of replacing returns duplicates.
- Publishes are casts; the client reads the relay's verdict from `OK`.
- Reconnect backoff is 1 s doubling to 60 s. Mint's connect timeout is capped at 5 s.

## Decisions made

* `2026-09-02` — **khatru, not a turnkey relay.** The requirement is NIP-42 gating of *reads and writes* by allowlist. strfry gates writes only (write-policy plugin) and nostr-rs-relay's `pubkey_whitelist` covers event publishing only; both would need a fork. khatru makes read gating a `RejectFilter` hook. Building from scratch was rejected: replaceable-event semantics, subscription bookkeeping and the NIP-42 state machine are the parts worth not rewriting. (Design conversation in the app repo, 2026-09-02.)
* `2026-09-02` — **One module, `fiatjaf.com/nostr`, pinned to `v0.0.0-20260902034142-316ef6591fa2`.** khatru is the package `fiatjaf.com/nostr/khatru` inside the module `fiatjaf.com/nostr` (source `gitnostr.com/.../nostrlib.git`), not a module of its own. `go get fiatjaf.com/nostr/khatru@latest` fails; pin the parent module. The same module carries the client (`nostr.Relay`), `nip11`, `nip42` and `eventstore`. The archived `github.com/fiatjaf/khatru` v0.19.1 is the fallback.
* `2026-09-02` — **bbolt eventstore, not SQLite** ([ADR-001](../decisions/001-bbolt-event-store.md)). The live module ships no SQLite backend (bleve, boltdb, lmdb, mmm, nullstore, slicestore only). bbolt keeps the properties the SQLite choice was made for: one file, pure Go, no service dependency, no cgo. Volume is bounded by members × titles.
* `2026-09-02` — **Hooks wired directly, not through `UseEventstore`.** `HandleNIP11` advertises NIP-9 whenever `DeleteEvent` is set and NIP-45 whenever `Count` is set, and `UseEventstore` sets both. Setting only `QueryStored`, `StoreEvent` and `ReplaceEvent` keeps the advertised list at 1, 11, 42 and leaves `COUNT` answered with `unsupported:` and deletion handled by the kind filter.
* `2026-09-02` — **Race detector on, `checkptr` off for the upstream module.** `go test -race` enables `checkptr`, which aborts in `fiatjaf.com/nostr`'s `writeJSONString` (uintptr arithmetic over string bytes; in bounds, but not provable to the checker). `scripts/check` passes `-gcflags='fiatjaf.com/nostr=-d=checkptr=0'` so the race detector still covers this repo's code.
* `2026-09-02` — **Test client is a raw WebSocket client, not `nostr.Relay`.** The module's client performs AUTH from its reader goroutine, races its challenge field under the race detector, and exposes no way to wait for the AUTH `OK`. The raw client in `client_test.go` mirrors the app's sequence exactly.
* `2026-09-02` — **`service_url` is required.** Without it khatru derives the relay URL from `Host` and `X-Forwarded-*` headers, which trusts the proxy and breaks silently when the proxy omits them. The operator states the URL members paste.
* `2026-09-02` — **Config file from step 1.** `name`, `listen`, `database` are read from the TOML file in step 1 so step 2 adds `members` to a working loader instead of replacing flags.
* `2026-09-02` — **Allowlist in a TOML config file**, one list of `npub` strings. Restart to apply in v1.
* `2026-09-02` — **Challenge on connect** (`RequestAuth` from `OnConnect`), because the client answers challenges proactively and never reacts to `auth-required:` rejections.
* `2026-09-02` — **Kinds accepted: 32160 only.** The auth kind 22242 is handled by the framework. Anything else is rejected with a `blocked:` reason.
* `2026-09-02` — **TLS terminates at the operator's reverse proxy.** The relay listens on plain HTTP on loopback or a container network. Documented, not built.
* `2026-09-02` — **Built in this repo by its own Claude Code instance.** The app instance seeds the docs and owns app-side changes; this instance owns the Go code. The app's skills, memory and `CLAUDE.md` are Elixir-specific and would mis-route work here.

## Next steps

1. ~~Done 2026-09-02.~~ `go mod init github.com/media-centaur/social-relay`; pin `fiatjaf.com/nostr`; a relay that reads `name`, `listen`, `database` from a TOML file, starts, serves a NIP-11 document naming itself and its supported NIPs (1, 11, 42), and stores kind 32160 events with replace semantics. `scripts/check` (staticcheck as a `go.mod` tool dependency) and CI (check on push) land in the same step.
2. ~~Done 2026-09-02.~~ `members` in the config and NIP-42 gating via `OnRequest` and `OnEvent` (khatru's hooks are single functions, not slices; `RequestAuth(ctx)` from `OnConnect`; `GetAuthed(ctx)` reads the socket's authenticated key). Tests through a real client on loopback: unauthenticated `REQ` gets `CLOSED` with `auth-required:`; a non-member authenticates then gets `CLOSED` / `OK false` with `restricted:`; a member round-trips an event; a second event with the same author and `d` replaces the first and a `REQ` returns only the newer one; `EOSE` follows stored matches.
3. ~~Done 2026-09-02.~~ Kind restriction. Kind 1 rejected with `blocked:`; kind 32160 accepted. `acceptedKinds` in `kinds.go` is the single config point.
4. ~~Done 2026-09-02.~~ Service URL check (`service_url`, required, ws or wss). khatru sets `Relay.ServiceURL` and compares through `nip42.ValidateAuthEvent`, which lowercases both sides, strips one trailing slash, and requires equal scheme, host and path. Setting `ServiceURL` also restricts the WebSocket and NIP-11 handlers to that path. Make it the config key `service_url`; test trailing slash, case, and `ws://` vs `wss://` (must differ).
5. ~~Done 2026-09-02.~~ Container image (scratch, non-root; config bind-mounted read-only at `/etc/social-relay/relay.toml`, database on the `/data` volume), `deploy/` compose example with Caddy, `docs/operating.md`, `scripts/build-release`, `.github/workflows/release.yml`.
6. ~~Done 2026-09-02.~~ End to end against the dev app on `:2160`, then write `docs/protocol.md` and cross-check it against `docs/social.md` line by line. Observed: the app stores a URL typed without a trailing slash with one (`ws://127.0.0.1:2173/`) and authenticates twice on connect; both accepted.
7. Tag a release (set `Info.Version` from build info; it reads `n/a` today); wiki page *Hosting a private relay* in `../media-centaur.wiki`.

## Open questions

1. ~~How a non-member's rejection surfaces in the app.~~ **Resolved 2026-09-02.** khatru's `AUTH` handler validates challenge, `relay` tag, timestamp and signature, then answers `OK true` and calls `OnAuth`, which is notify-only. There is no hook that can refuse the auth event. A non-member therefore authenticates successfully and is refused on the first `REQ` (`CLOSED restricted:`) and every `EVENT` (`OK false restricted:`). Moved to **Cross-repo**.
2. **Allowlist changes without restart.** Restart is acceptable for a friend group. Revisit only if operators ask.
3. **Deletion and retention.** The app never sends kind 5. Reject it with `blocked:` for now. Retention is unnecessary while storage is bounded by addressable events.
4. **Directed recommendations later** (NIP-17 gift wraps, kind 1059) widen the kind allowlist and add recipient-scoped read rules. Not v1; the kind list must be a single config point so this is a one-line change.

## Cross-repo

Items that need an app-side change. Move each to the app campaign `../media-centaur-app/campaigns/friends-recommendations.md` when it becomes concrete.

* **Non-member rejection surfaces as `last_error`, not as *Relay rejected this identity*.** The relay cannot refuse `AUTH` (open question 1). Exact wire behaviour, fixed in `docs/protocol.md`: `AUTH` → `OK true`; then `["CLOSED", "feed", "restricted: this key is not a member of this relay"]` and the same on `own:<url>`; every `EVENT` → `OK false` with the same reason. Proposed app change: treat `CLOSED restricted:` on `feed` from a relay that accepted the `AUTH` as `auth_failed`. Ready to move to `../media-centaur-app/campaigns/friends-recommendations.md`.

## Completion criteria

* `docker run` with a mounted config file yields a relay that challenges on connect, refuses `REQ` and `EVENT` from non-members, stores kind 32160 with replace semantics, and rejects every other kind.
* A recommendation sent from the dev app is received by a second client (a second app instance or a scripted go-nostr client) through this relay, with no app-side change.
* A tagged release with a container image on GHCR, and the wiki page.
* `docs/protocol.md` exists and agrees with `docs/social.md`.

## Pointers

* App contributor guide: `../media-centaur-app/docs/social.md`.
* App glossary: `../media-centaur-app/docs/GLOSSARY.md`.
* App design spec: `../media-centaur-app/docs/superpowers/specs/2026-09-02-friends-recommendations-design.md`.
* App campaign: `../media-centaur-app/campaigns/friends-recommendations.md`.
* App test relay (the subset the client uses, in Elixir): `../media-centaur-app/test/support/nostr/fake_relay.ex`.
* khatru: https://pkg.go.dev/fiatjaf.com/nostr/khatru (live), https://github.com/fiatjaf/khatru (archived). Local copy of the pinned module: `$(go env GOMODCACHE)/fiatjaf.com/nostr@v0.0.0-20260902034142-316ef6591fa2/`; `khatru/handlers.go` is the message loop, `khatru/docs/` the cookbook.
* NIPs: 01 (events, filters, `OK`/`CLOSED` prefixes), 11 (relay information document), 42 (client authentication).
