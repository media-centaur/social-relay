---
status: accepted
date: 2026-09-04
---
# ADR-002: The address slot is enforced by the relay, not by khatru's deletion handler

## Context and Problem Statement

The app's contract (`docs/social-protocol.md` in the app repository) requires one record per signer per address: a recommendation (kind 32160) or the deletion (kind 5) that withdrew it, never both and never two of either, with `created_at` deciding which one stands. khatru has a built-in NIP-09 path: it stores every kind 5 as a regular event through `StoreEvent`, then queries and deletes the targets through `DeleteEvent`. Where should the slot rule live?

## Decision Drivers

* The contract's rules 2, 3 and 4 must hold exactly, including the tie cases.
* One place decides ordering, so a reader can verify the rules against one routine.
* No fork of khatru and no second index beside the event store.

## Considered Options

* Set `DeleteEvent` and let khatru's handler remove recommendations, with `OnEvent` checking the deletion's shape.
* Replace `StoreEvent` and `ReplaceEvent` with one routine of the relay's own that reads the slot and writes it under a mutex; leave `DeleteEvent` unset.

## Decision Outcome

Chosen option: the relay's own routine (`internal/relay/slot.go`). khatru's handler stores a kind 5 before it looks at the targets, so two deletions for one address would both persist, and a deletion older than the stored recommendation would persist beside it. Its handler also cannot refuse a recommendation because of a stored deletion (rule 4). Wrapping it would spread the rule over three hooks. The tombstone is the stored kind 5 event itself, found by `authors` and `#a`; no side index.

### Consequences

* Good, because every ordering rule, tie included, is in one function pair with one test file against it.
* Good, because discarded events are answered `OK true` without a broadcast, which khatru's replace path did not do.
* Bad, because NIP-9 must be added to `supported_nips` by hand, since khatru advertises it only when `DeleteEvent` is set.
* Bad, because writes are serialised by a mutex in the relay rather than by one store transaction; the store's exported API opens a transaction per call. A friend group's write rate makes this irrelevant.
