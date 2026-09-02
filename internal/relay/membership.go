package relay

import (
	"context"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/khatru"
)

// Rejection reasons. These strings are part of the client contract in docs/protocol.md;
// the app matches on the NIP-01 prefix before the colon.
const (
	reasonAuthRequiredRead  = "auth-required: authenticate to read from this relay"
	reasonAuthRequiredWrite = "auth-required: authenticate to write to this relay"
	reasonNotMember         = "restricted: this key is not a member of this relay"
	reasonAuthorNotMember   = "restricted: the event author is not a member of this relay"
)

// membership answers khatru's OnRequest and OnEvent hooks from the allowlist.
type membership map[nostr.PubKey]struct{}

func newMembership(members []nostr.PubKey) membership {
	m := make(membership, len(members))
	for _, pk := range members {
		m[pk] = struct{}{}
	}
	return m
}

func (m membership) contains(pk nostr.PubKey) bool {
	_, ok := m[pk]
	return ok
}

// challenge sends the NIP-42 AUTH challenge as soon as the socket opens. The app
// answers challenges proactively and never reacts to auth-required: rejections.
func (m membership) challenge(ctx context.Context) {
	khatru.RequestAuth(ctx)
}

func (m membership) onRequest(ctx context.Context, _ nostr.Filter) (reject bool, msg string) {
	pk, authed := khatru.GetAuthed(ctx)
	switch {
	case !authed:
		return true, reasonAuthRequiredRead
	case !m.contains(pk):
		return true, reasonNotMember
	}
	return false, ""
}

func (m membership) onEvent(ctx context.Context, event nostr.Event) (reject bool, msg string) {
	pk, authed := khatru.GetAuthed(ctx)
	switch {
	case !authed:
		return true, reasonAuthRequiredWrite
	case !m.contains(pk):
		return true, reasonNotMember
	case !m.contains(event.PubKey):
		return true, reasonAuthorNotMember
	}
	return false, ""
}
