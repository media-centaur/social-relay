# social-relay

The private relay for [Media Centaur](https://github.com/media-centaur/media-centaur)'s Social feature. One friend group runs one instance. Only the public keys listed in its config file can read or write, and it stores only Media Centaur's recommendation events.

- **Run one:** [docs/operating.md](docs/operating.md), or the wiki page [Hosting a private relay](https://github.com/media-centaur/media-centaur/wiki/Hosting-a-Private-Relay).
- **What it speaks:** [docs/protocol.md](docs/protocol.md), the relay's side of the contract with the app.
- **Work on it:** [CLAUDE.md](CLAUDE.md) is the contributor guide; `scripts/check` is the gate.

Container image: `ghcr.io/media-centaur/social-relay`. Binaries: [releases](https://github.com/media-centaur/social-relay/releases).

MIT licensed.
