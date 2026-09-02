> **Internal contributor guide.** Orientation for working *on* this repository, human or AI. Operators read [README.md](README.md) (to be written).

# social-relay

The allowlist Nostr relay for Media Centaur's friend network. A friend group runs one instance; each member pastes its URL into Media Centaur's Friends tab; only allowlisted public keys can read or write, and only the event kinds Media Centaur uses are stored. This is the **private relay** half of the app's "control on a slider" design. Public relays are the other half and are not this repo's concern.

The client is `MediaCentaur.Nostr` and `MediaCentaur.Friends` in the sibling repo `../media-centaur-app`. Its side of the contract is documented in `../media-centaur-app/docs/friends.md` (sections Transport, Event shape, Sync). This repo's side lives in `docs/protocol.md` once written. The two must agree; when one changes, change the other in the same unit of work.

## Stack

- Go, one static binary, built on **khatru**, the package `fiatjaf.com/nostr/khatru` inside the single module `fiatjaf.com/nostr` (which also provides the client, `nip11`, `nip42` and `eventstore`). The GitHub repository `github.com/fiatjaf/khatru` was archived in January 2026; the live module is published as Go pseudo-versions from the author's own git host, so **pin an exact pseudo-version** and treat every upgrade as a potential API break.
- Event storage: the bbolt backend `fiatjaf.com/nostr/eventstore/boltdb` (ADR-001). One file, pure Go.
- Distribution: a container image on GHCR plus the bare binary attached to a GitHub release. TLS is the operator's reverse proxy, not the relay.

## Working in this repo

- **Campaigns first.** `campaigns/` holds one markdown per multi-session initiative, same convention as the app (ADR-042 there). When resuming one, reconcile the file against `git log` and the code and update it *before* writing new code.
- **Test-first.** Relay behaviour is specified by tests that open a real WebSocket to an in-process relay on a loopback port. Nothing in the suite touches the network beyond loopback.
- **`scripts/check`** is the gate: build, `go vet`, `staticcheck`, `go test`. Define it before the first feature lands, and run it before finishing any change. Zero warnings.
- **Scripts** are extensionless executables with shebangs under `scripts/`.
- **Decisions** go in `decisions/` as MADR 4.0 records, cited as ADR-NNN, numbered independently of the app's.
- **Fixtures** generate keypairs at test time. No real keys, no real show titles, no real people. Generic placeholders only.
- **Terminology** is defined in the campaign's glossary before first use. Established Nostr terms (relay, event, kind, filter, subscription, NIP) are used as the NIPs use them.

## Cross-repo verification

The app's dev server runs at `http://localhost:2160` (systemd user unit `media-centaur-dev`, real database). End-to-end check for any change that touches authentication or acceptance: run the relay locally, add `ws://127.0.0.1:<port>` on the app's Friends tab, and confirm the relay log shows the app's public key authenticating and the app's Status page Friends tile reads connected.

A change that alters what the relay accepts, how it authenticates, or what it answers on rejection is a cross-repo change. Record it in the campaign under **Cross-repo** so the app-side work is not lost.
