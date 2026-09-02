# Campaigns

One markdown per multi-session initiative, removed when complete. Same convention as the app repository (`../media-centaur-app/campaigns/README.md`, ADR-042 there): frontmatter `status` / `started` / `last_updated`, sections Goal / Glossary / Status / Decisions made / Next steps / Completion criteria, and the reconciliation rule on resume.

## Active

* [`private-relay-v1.md`](private-relay-v1.md) — **shipped as v0.1.0; wiki push and GHCR visibility left for the owner.** The first shippable relay: one container, a config file of member keys, NIP-42 gating of reads and writes, kind 32160 only, verified end to end against the dev app.
