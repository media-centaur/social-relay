---
status: accepted
date: 2026-09-02
---
# ADR-001: bbolt event store

## Context and Problem Statement

The relay needs an event store from khatru's `eventstore` family. The campaign was seeded with "the SQLite backend", but the live module `fiatjaf.com/nostr` (pinned `v0.0.0-20260902034142-316ef6591fa2`) ships only bleve, boltdb, lmdb, mmm, nullstore and slicestore. The SQLite backend existed only in the archived `github.com/fiatjaf/eventstore`, whose types are incompatible with the live khatru.

## Decision Drivers

* One file on a volume, no service dependency, so a friend group can run one container.
* Pure Go, so the binary is static and builds without a C toolchain.
* Storage volume is bounded by members × titles; performance is not a driver.

## Considered Options

* boltdb (`go.etcd.io/bbolt`): pure Go, one file.
* lmdb: one directory, requires cgo.
* mmm: memory-mapped custom format, more moving parts than the volume justifies.
* Archived khatru plus archived SQLite eventstore: unmaintained.

## Decision Outcome

Chosen option: boltdb, because it is the only option that satisfies both drivers on the live module without a fork or an unmaintained dependency.

### Consequences

* Good, because the data directory holds a single file and backups are a file copy while the relay is stopped.
* Bad, because there is no SQL shell for ad-hoc inspection; the module's `eventstore` command-line tool is the inspection path.
