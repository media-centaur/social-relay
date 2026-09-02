# Protocol

The relay's side of the contract with Media Centaur. The app's side is `docs/social.md` in the app repository, sections Transport, Event shape and Sync. When one changes, change the other in the same unit of work.

The relay implements NIP-01 (events, filters, subscriptions), NIP-11 (relay information document) and NIP-42 (client authentication), and nothing else. Every rejection reason below is a fixed string; clients match on the prefix before the colon, as NIP-01 specifies.

## Endpoint

One address, the configured `service_url`, serves everything.

| Request | Response |
|---|---|
| WebSocket upgrade | The NIP-01 connection. |
| `GET` with `Accept: application/nostr+json` | The NIP-11 document: `name`, `supported_nips` `[1, 11, 42]`, `software`, `version`. |
| Any other path | `404`. Only the path of `service_url` is served. |

TLS terminates at the operator's reverse proxy. The relay itself speaks plain HTTP.

## Connection sequence

1. The client opens the WebSocket. The relay sends `["AUTH", "<challenge>"]` before the client has said anything.
2. The client answers with `["AUTH", <event>]`, a kind `22242` event carrying a `relay` tag and a `challenge` tag, signed by the key it wants to act as. The relay checks, in order: kind, challenge, `relay` tag, `created_at` within ten minutes of the relay's clock, signature.
3. The relay answers `["OK", "<id>", true, ""]` or `["OK", "<id>", false, "error: failed to authenticate: <detail>"]`.
4. The client sends `REQ` and `EVENT` messages. Every one of them is judged against the key that authenticated last on this socket.

The `relay` tag is compared with `service_url` after lowercasing both and removing one trailing slash; scheme, host and path must then be equal. So `ws://` against `wss://` fails, a path present on one side only fails, and `WSS://Relay.Example/` against `wss://relay.example` passes. The app has been observed to store a URL typed without a trailing slash as `ws://127.0.0.1:2173/`; that passes.

Authentication succeeds for any key with a valid signature, member or not. Membership is enforced on `REQ` and `EVENT`, not on `AUTH`: khatru offers no hook to refuse an `AUTH` event. A second `AUTH` on the same socket is accepted and the newest key becomes the one that is checked.

A `REQ` or `EVENT` that arrives before authentication is refused with an `auth-required:` reason, and the relay sends the `AUTH` challenge again.

## REQ

| Situation | Answer |
|---|---|
| Not authenticated | `["CLOSED", "<sub>", "auth-required: authenticate to read from this relay"]` |
| Authenticated, key not in `members` | `["CLOSED", "<sub>", "restricted: this key is not a member of this relay"]` |
| Member | Every stored event matching the filter as `["EVENT", "<sub>", <event>]`, then `["EOSE", "<sub>"]`. The subscription stays open and later matching events are pushed as they are stored. |

Filters are NIP-01 filters: `ids`, `authors`, `kinds`, `#<tag>`, `since`, `until`, `limit`. A `limit` above 1000 is capped. A `REQ` with several filters is rejected as a whole if any filter is rejected. Reusing a subscription id replaces the earlier subscription with that id on the same socket, so a client that re-issues `feed` after reconnecting or after authenticating does not accumulate duplicates. `COUNT` is answered with `["CLOSED", "<sub>", "unsupported: this relay does not support NIP-45"]`.

## EVENT

The verdicts, in the order they are checked. The first that applies is the answer.

| Situation | Answer |
|---|---|
| `id` does not match the content | `["OK", "<id>", false, "invalid: id is computed incorrectly"]` |
| Signature invalid | `["OK", "<id>", false, "invalid: signature is invalid"]` |
| Not authenticated | `["OK", "<id>", false, "auth-required: authenticate to write to this relay"]` |
| Authenticated, key not in `members` | `["OK", "<id>", false, "restricted: this key is not a member of this relay"]` |
| Event's `pubkey` not in `members` | `["OK", "<id>", false, "restricted: the event author is not a member of this relay"]` |
| Kind other than `32160` | `["OK", "<id>", false, "blocked: kind <n> is not stored by this relay"]` |
| Stored | `["OK", "<id>", true, ""]` |

Kind `32160` is addressable, so the relay keeps one event per `(pubkey, d tag)`. A newer event replaces the stored one; an event older than the stored one is answered `OK true` and discarded; an event whose `id` is already stored is answered `OK true`. Stored events are pushed to every open subscription whose filter matches.

The relay never reads the content. The `d` tag layout `tmdb:<media_type>:<tmdb_id>` and the content JSON are the app's business.

## Kinds

| Kind | Handling |
|---|---|
| `22242` | Consumed by the authentication handshake. Never stored. |
| `32160` | Stored with replace semantics. |
| Everything else, including `5` (deletion) | `blocked:`. The relay does not implement NIP-9; recommendations are replaced, never deleted. |

Widening the list is one edit to `acceptedKinds` in `internal/relay/kinds.go`.

## Cross-check against the app

What `docs/social.md` says the app does, and how the relay answers it.

| App | Relay |
|---|---|
| Answers any `AUTH` challenge immediately with the URL exactly as the user configured it in the `relay` tag. Never re-authenticates in response to an `auth-required:` rejection. | Challenges on connect, so the app never needs to react to a rejection. Compares the tag as described above. |
| Re-issues every subscription after a successful `AUTH`. | A `REQ` sent before the `AUTH` `OK` is `CLOSED auth-required:`; the re-issue after `OK` succeeds. |
| Treats `OK false` for the auth event as `auth_failed`, shown as **Rejected**. | Happens only for a malformed `AUTH` or a `relay` tag that does not match `service_url`. A non-member's `AUTH` succeeds. |
| Folds a `CLOSED` on any subscription into the relay row's `last_error`; the row stays **Connected**. | This is where a non-member lands: `restricted:` on `feed` and `own:<url>`. See below. |
| On `EOSE` for `own:<url>`, publishes every stored own event the relay did not return. | `EOSE` follows exactly the stored set for the filter, and replace semantics mean the set holds one event per title, so nothing is republished needlessly and nothing comes back twice. |
| Sends `feed` and `own:<url>` twice after a reconnect. | The second `REQ` with the same id replaces the first. |
| Reconnects with backoff from 1 s to 60 s. | A relay restart closes every socket; members are back within a minute. |
| Publishes are casts; the verdict is read from `OK`. | Every `EVENT` gets exactly one `OK`. |

**Open on the app side:** a member whose key is not on the list sees **Connected** with *restricted: this key is not a member of this relay* as the row's last error, not **Rejected**. The relay cannot make that an authentication failure. The app could treat `CLOSED restricted:` on `feed` from a relay that accepted its `AUTH` as one.
