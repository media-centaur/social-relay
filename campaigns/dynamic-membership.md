---
status: shipped
started: 2026-09-02
last_updated: 2026-09-02
---
# Dynamic membership

## Goal

Adding or removing a member must not require a restart or a redeploy. With v0.1.0 the member list lives in the config file; under a deploy-by-commit setup that makes every new friend a commit, a build and a container recreate. The member list is data, not configuration, and moves into the database, changed at runtime through the standard relay management API.

## Glossary

- **Admin** — a public key named in the config file under `admins`. Admins may manage membership and are members themselves. The only static access-control input.
- **Member** — a public key allowed to read and write: every admin, plus every key an admin has allowed. Allowed keys live in the database.
- **Management API** — NIP-86: signed JSON-RPC over HTTP on the relay's own address, authenticated per NIP-98. The relay implements `allowpubkey`, `unallowpubkey` and `listallowedpubkeys`.
- **Management client** — the `social-relay members` subcommand, which signs management requests with an admin's secret key.

## Status

Shipped 2026-09-02 as v0.2.0. Steps 1 to 3 done; the CLI was smoke-tested against a live relay (add, list, remove, usage error, non-admin refusal). Delete this file once the release workflow is green and the cross-repo item has moved.

## Decisions made

* `2026-09-02` — **`admins` replaces `members` in the config.** One representation of access control input. A static group lists everyone as an admin and never touches the API. No compatibility path for `members`; the loader rejects it as an unknown key with the usual error.
* `2026-09-02` — **Allowed keys live in a bucket of the existing bbolt file.** One file to back up; the eventstore ignores buckets it does not own. The set is held in memory behind a mutex and written through.
* `2026-09-02` — **Only `allowpubkey`, `unallowpubkey`, `listallowedpubkeys`.** No ban list: an allowlist relay has one set. `listallowedpubkeys` includes admins, with the reason `admin`, so it reports the truth. Unallowing an admin is an error pointing at the config.
* `2026-09-02` — **Every management method requires an admin.** `OnAPICall` rejects everyone else with `restricted:`.
* `2026-09-02` — **Removal takes effect on open connections.** Membership is checked per `REQ` and `EVENT` already; live delivery to an unallowed key's open subscriptions is stopped through khatru's `PreventBroadcast`.
* `2026-09-02` — **CLI reads the admin key from `SOCIAL_RELAY_ADMIN_KEY`** (nsec or hex), never from a flag, so it does not land in shell history; `op run` or the like supplies it.

## Next steps

1. ~~Done 2026-09-02.~~ Tests: config `admins`; allow then read and write; unallow then `restricted:` and no live delivery; non-admin rejected; persistence across restart; list includes admins; unallowing an admin fails; NIP-11 lists 86.
2. ~~Done 2026-09-02.~~ Implementation: member store, management hooks, `PreventBroadcast`, `internal/manage` client, `members add|remove|list` subcommand.
3. ~~Done 2026-09-02.~~ Docs: `docs/protocol.md` management section, `docs/operating.md`, README, `relay.example.toml`, `deploy/relay.toml`, `scripts/dev-relay` writes `admins`, wiki page.
4. Release v0.2.0 (tag pushed 2026-09-02).

## Cross-repo

* Media Centaur holds the operator's identity and roster. With NIP-86 on the relay, the app can allow a friend on relays where the identity is an admin, at "Add friend" time or by a control beside the friend. Move to the app campaign once v0.2.0 is tagged.

## Completion criteria

* An admin adds a member with one command while the relay runs; the member's Media Centaur turns from *restricted* to reading the feed without reconnecting.
* Removing a member stops their reads, writes and live delivery at once.
* Members survive a relay restart.
* v0.2.0 tagged with image and binaries.
