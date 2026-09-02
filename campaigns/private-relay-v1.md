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

In progress. Step 1 done 2026-09-02: module pinned, bbolt store, TOML config (`name`, `listen`, `database`), NIP-11 document advertising 1/11/42, addressable replace semantics, `scripts/check` with race detector, CI workflow. Not yet committed at the time of writing. Step 2 (members and NIP-42 gating) is next.

Layout: `cmd/social-relay` (flag `-config`, graceful shutdown), `internal/relay` (`Config`/`LoadConfig`, `New`, the khatru wiring), tests in `internal/relay/*_test.go` through `httptest.NewServer` and the module's own `nostr.Relay` client.

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
* `2026-09-02` — **Config file from step 1.** `name`, `listen`, `database` are read from the TOML file in step 1 so step 2 adds `members` to a working loader instead of replacing flags.
* `2026-09-02` — **Allowlist in a TOML config file**, one list of `npub` strings. Restart to apply in v1.
* `2026-09-02` — **Challenge on connect** (`RequestAuth` from `OnConnect`), because the client answers challenges proactively and never reacts to `auth-required:` rejections.
* `2026-09-02` — **Kinds accepted: 32160 only.** The auth kind 22242 is handled by the framework. Anything else is rejected with a `blocked:` reason.
* `2026-09-02` — **TLS terminates at the operator's reverse proxy.** The relay listens on plain HTTP on loopback or a container network. Documented, not built.
* `2026-09-02` — **Built in this repo by its own Claude Code instance.** The app instance seeds the docs and owns app-side changes; this instance owns the Go code. The app's skills, memory and `CLAUDE.md` are Elixir-specific and would mis-route work here.

## Next steps

1. ~~Done 2026-09-02.~~ `go mod init github.com/media-centaur/social-relay`; pin `fiatjaf.com/nostr`; a relay that reads `name`, `listen`, `database` from a TOML file, starts, serves a NIP-11 document naming itself and its supported NIPs (1, 11, 42), and stores kind 32160 events with replace semantics. `scripts/check` (staticcheck as a `go.mod` tool dependency) and CI (check on push) land in the same step.
2. `members` in the config and NIP-42 gating via `OnRequest` and `OnEvent` (khatru's hooks are single functions, not slices; `RequestAuth(ctx)` from `OnConnect`; `GetAuthed(ctx)` reads the socket's authenticated key). Tests through a real client on loopback: unauthenticated `REQ` gets `CLOSED` with `auth-required:`; a non-member authenticates then gets `CLOSED` / `OK false` with `restricted:`; a member round-trips an event; a second event with the same author and `d` replaces the first and a `REQ` returns only the newer one; `EOSE` follows stored matches.
3. Kind restriction. Kind 1 rejected with `blocked:`; kind 32160 accepted.
4. Service URL check. khatru sets `Relay.ServiceURL` and compares through `nip42.ValidateAuthEvent`, which lowercases both sides, strips one trailing slash, and requires equal scheme, host and path. Setting `ServiceURL` also restricts the WebSocket and NIP-11 handlers to that path. Make it the config key `service_url`; test trailing slash, case, and `ws://` vs `wss://` (must differ).
5. Container image (scratch, non-root, config and database on a volume), a compose example with a reverse proxy, `docs/operating.md`.
6. End to end against the dev app on `:2160`, then write `docs/protocol.md` and cross-check it against `docs/social.md` line by line.
7. Tag a release (set `Info.Version` from build info; it reads `n/a` today); wiki page *Hosting a private relay* in `../media-centaur.wiki`.

## Open questions

1. ~~How a non-member's rejection surfaces in the app.~~ **Resolved 2026-09-02.** khatru's `AUTH` handler validates challenge, `relay` tag, timestamp and signature, then answers `OK true` and calls `OnAuth`, which is notify-only. There is no hook that can refuse the auth event. A non-member therefore authenticates successfully and is refused on the first `REQ` (`CLOSED restricted:`) and every `EVENT` (`OK false restricted:`). Moved to **Cross-repo**.
2. **Allowlist changes without restart.** Restart is acceptable for a friend group. Revisit only if operators ask.
3. **Deletion and retention.** The app never sends kind 5. Reject it with `blocked:` for now. Retention is unnecessary while storage is bounded by addressable events.
4. **Directed recommendations later** (NIP-17 gift wraps, kind 1059) widen the kind allowlist and add recipient-scoped read rules. Not v1; the kind list must be a single config point so this is a one-line change.

## Cross-repo

Items that need an app-side change. Move each to the app campaign `../media-centaur-app/campaigns/friends-recommendations.md` when it becomes concrete.

* **Non-member rejection surfaces as `last_error`, not as *Relay rejected this identity*.** The relay cannot refuse `AUTH` (open question 1). The app should treat `CLOSED restricted:` on the `feed` subscription from a relay that has accepted its `AUTH` as an authentication failure and raise the incident. Concrete once step 2 lands and the exact `CLOSED` reason string is fixed in `docs/protocol.md`.

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
