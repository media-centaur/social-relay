---
status: ready
started: 2026-09-02
last_updated: 2026-09-02
---
# Deletion and sync v1

## Goal

The relay honours the app's wire contract as written in the app repository's `docs/social-protocol.md` (wiki page *Social Protocol*), which now defines deletion (kind 5), one-record-per-address semantics that include deletions, incremental and paged reads, and a request ceiling. Today the relay refuses kind 5 and has no `limit` cap; the app cannot withdraw a recommendation. Agreed between the owner and the app instance on 2026-09-02; the contract page is the source of truth, this file is the relay's to-do list against it.

## Glossary

- **Address** — `<kind>:<pubkey>:<d>`; for a recommendation `32160:<pubkey>:tmdb:<media_type>:<tmdb_id>`.
- **Deletion** — a kind 5 event (NIP-09) whose single `a` tag names the signer's own recommendation address.
- **Tombstone** — the stored deletion standing in the address's slot after the recommendation is removed.

## Deliverables

1. **Accept kind 5** from members, restricted to the address form: exactly one `a` tag, kind `32160`, the pubkey in the tag equal to the event's `pubkey`. Anything else: `OK false blocked: only the author may delete an event`. `e` tags are accepted and ignored; `content` is ignored.
2. **Apply it**: remove the addressed recommendation if its `created_at` is at or before the deletion's (protocol rule 2); store the deletion in the address's slot (rule 3). One record per signer per address, recommendation or deletion, never both. A deletion for an address the relay never held is `OK true`.
3. **Refuse resurrection**: a kind 32160 whose address holds a newer deletion is `OK false blocked: a newer deletion exists for this address` (rule 4). A newer recommendation replaces the deletion.
4. **Serve deletions** on `REQ` like recommendations: `authors`, `kinds` (5 alone or with 32160), `since`, `until`, `limit`. `EOSE` after stored matches.
5. **Cap `limit` at 500**; a `REQ` asking for more gets 500.
6. **Reads for members only**, as today; a non-member's `REQ` for kind 5 is `CLOSED restricted:` like any other.
7. `docs/protocol.md` updated to point at the app's contract page for the shared rules and to state only what is relay-specific (membership, administration, rejection strings), so the two never restate each other. The rejection strings above are the contract's; keep them byte-identical.
8. Tests through the raw WebSocket client: author deletes own recommendation → `REQ` for 32160 returns nothing, `REQ` for 5 returns the deletion; a member deleting another's address is refused; an older recommendation after a deletion is refused; a newer one replaces the deletion and a `REQ` for 5 no longer returns it; `limit` 1000 returns 500.

## Notes for the implementer

- khatru handles kind 5 in its message loop when `DeleteEvent` is set; check whether its built-in path enforces author match and `created_at` ordering for `a`-tag deletions or whether that belongs in `OnEvent`. The relay currently avoids `UseEventstore` to keep NIP-11 honest (ADR in the first campaign); wiring `DeleteEvent` adds NIP-9 to the advertised list, which is now correct.
- The bbolt store's replaceable-event handling must treat a deletion as occupying the address, so a later older recommendation can be compared against it. If the store cannot express "a kind 5 holds the 32160 slot", keep a small tombstone index keyed by address with the deletion's `created_at` and id.
- The app already publishes deletions and tombstones locally against `FakeRelay`; the first end-to-end check is `just social-recommend` from the app repo, delete it in the app's Recommendations tab, then `just social-feed` shows the kind 5 and not the recommendation.

## Completion criteria

- Deliverables 1–6 pass their tests (8) under `scripts/check`.
- `docs/protocol.md` defers to the contract page (7).
- A recommendation withdrawn in the dev app disappears from `just social-feed` and from a second member's feed after reconnect.
- Tagged release; the app's campaign notes the version the contract needs.
