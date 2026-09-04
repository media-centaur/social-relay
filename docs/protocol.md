# Protocol

The relay's side of the contract with Media Centaur. The shared rules live on the app's contract page, `docs/social-protocol.md` in the app repository (wiki page *Social Protocol*): kind numbers, the shape of a recommendation (kind 32160) and a deletion (kind 5), the address-slot rules, and the subscriptions and paging the app performs. This page states only what the relay adds: membership, administration, and the order in which verdicts are reached. When one changes, change the other in the same unit of work.

The relay implements NIP-01 (events, filters, subscriptions), NIP-09 (deletion, address form only), NIP-11 (relay information document), NIP-42 (client authentication) and NIP-86 (relay management), and nothing else. Every rejection reason is a fixed string; clients match on the prefix before the colon, as NIP-01 specifies. The strings are the contract page's, byte for byte.

## Endpoint

One address, the configured `service_url`, serves everything.

| Request | Response |
|---|---|
| WebSocket upgrade | The NIP-01 connection. |
| `GET` with `Accept: application/nostr+json` | The NIP-11 document: `name`, `supported_nips` `[1, 9, 11, 42, 86]`, `software`, `version`. |
| `POST` with `Content-Type: application/nostr+json+rpc` | The NIP-86 management API, below. |
| Any other path | `404`. Only the path of `service_url` is served. |

TLS terminates at the operator's reverse proxy. The relay itself speaks plain HTTP.

## Connection sequence

1. The client opens the WebSocket. The relay sends `["AUTH", "<challenge>"]` before the client has said anything.
2. The client answers with `["AUTH", <event>]`, a kind `22242` event carrying a `relay` tag and a `challenge` tag, signed by the key it wants to act as. The relay checks, in order: kind, challenge, `relay` tag, `created_at` within ten minutes of the relay's clock, signature.
3. The relay answers `["OK", "<id>", true, ""]` or `["OK", "<id>", false, "error: failed to authenticate: <detail>"]`.
4. The client sends `REQ` and `EVENT` messages. Every one of them is judged against the key that authenticated last on this socket.

The `relay` tag is compared with `service_url` after lowercasing both and removing one trailing slash; scheme, host and path must then be equal. So `ws://` against `wss://` fails, a path present on one side only fails, and `WSS://Relay.Example/` against `wss://relay.example` passes.

Authentication succeeds for any key with a valid signature, member or not. A **member** is an admin named in the config file or a key an admin has allowed through the management API. Membership is enforced on `REQ` and `EVENT`, not on `AUTH`: khatru offers no hook to refuse an `AUTH` event. A second `AUTH` on the same socket is accepted and the newest key becomes the one that is checked.

A `REQ` or `EVENT` that arrives before authentication is refused with an `auth-required:` reason, and the relay sends the `AUTH` challenge again.

## REQ

| Situation | Answer |
|---|---|
| Not authenticated | `["CLOSED", "<sub>", "auth-required: authenticate to read from this relay"]` |
| Authenticated, key not a member | `["CLOSED", "<sub>", "restricted: this key is not a member of this relay"]` |
| Member | Every stored event matching the filter as `["EVENT", "<sub>", <event>]`, then `["EOSE", "<sub>"]`. The subscription stays open and later stored events are pushed as they arrive, for as long as the key stays a member. |

Filters are NIP-01 filters: `ids`, `authors`, `kinds`, `#<tag>`, `since`, `until`, `limit`. `limit` is capped at 500, the app's page size. A `REQ` with several filters is rejected as a whole if any filter is rejected. Reusing a subscription id replaces the earlier subscription with that id on the same socket. `COUNT` is answered with `["CLOSED", "<sub>", "unsupported: this relay does not support NIP-45"]`.

## EVENT

The verdicts, in the order they are checked. The first that applies is the answer.

| Situation | Answer |
|---|---|
| `id` does not match the content | `["OK", "<id>", false, "invalid: id is computed incorrectly"]` |
| Signature invalid | `["OK", "<id>", false, "invalid: signature is invalid"]` |
| Not authenticated | `["OK", "<id>", false, "auth-required: authenticate to write to this relay"]` |
| Authenticated, key not a member | `["OK", "<id>", false, "restricted: this key is not a member of this relay"]` |
| Event's `pubkey` not a member | `["OK", "<id>", false, "restricted: the event author is not a member of this relay"]` |
| Kind other than `32160` or `5` | `["OK", "<id>", false, "blocked: kind <n> is not stored by this relay"]` |
| Kind `5` without exactly one `a` tag of kind `32160` naming the signer's own pubkey | `["OK", "<id>", false, "blocked: only the author may delete an event"]` |
| Kind `32160` created at or before the deletion stored for its address | `["OK", "<id>", false, "blocked: a newer deletion exists for this address"]` |
| Stored, or discarded because the address holds something newer | `["OK", "<id>", true, ""]` |

**The address slot.** An address `32160:<pubkey>:<d>` holds one record: the signer's recommendation or the deletion that withdrew it, never both and never two of either. A newer `created_at` takes the slot from whatever holds it. On equal `created_at`, a deletion beats a recommendation (contract rule 2) and otherwise the stored record stays. A discarded event is answered `OK true` and is not pushed to open subscriptions, because the relay does not hold it. A deletion for an address the relay never held is stored as the tombstone.

The relay never reads content. The `d` tag layout and the content JSON are the app's business.

## Management

Admins change membership while the relay runs, through NIP-86: a `POST` to the relay's address with `Content-Type: application/nostr+json+rpc`, body `{"method": ..., "params": [...]}`, and an `Authorization: Nostr <base64 event>` header carrying a NIP-98 event (kind `27235`) signed by the admin. khatru verifies the event before any method runs: signature; `u` tag equal to `service_url` after URL normalisation; `payload` tag equal to the SHA-256 of the body; `created_at` within 30 seconds.

| Method | Params | Effect |
|---|---|---|
| `allowpubkey` | `[<hex pubkey>, <reason>]` | The key becomes a member at once, on open connections too. Allowing an admin or an existing member is a no-op. |
| `unallowpubkey` | `[<hex pubkey>, <reason>]` | The key stops being a member at once: its next `REQ` and `EVENT` are `restricted:`, and live events are no longer delivered to its open subscriptions. Unallowing an admin is an error; admins are set in the config file. |
| `listallowedpubkeys` | `[]` | Every member as `{"pubkey", "reason"}`, admins with the reason `admin`. |
| `supportedmethods` | `[]` | The three above. |

Every method, `supportedmethods` included, requires an admin. Anyone else receives `{"error": "restricted: only admins may manage this relay"}`. Allowed keys are stored in the relay's database and survive restarts.

The `social-relay members` subcommand is a client for this API; Media Centaur may become another.

## Open on the app side

A key that is not a member sees **Connected** with *restricted: this key is not a member of this relay* as the relay row's last error, not **Rejected**. The relay cannot make that an authentication failure. The app could treat `CLOSED restricted:` on `feed` from a relay that accepted its `AUTH` as one.
