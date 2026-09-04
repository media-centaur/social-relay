package relay

import (
	"context"
	"fmt"

	"fiatjaf.com/nostr"
)

// kindRecommendation is the app's addressable recommendation event. The relay stores it
// and never interprets it; its shape is the app's contract (docs/protocol.md).
const kindRecommendation nostr.Kind = 32160

// kindDeletion withdraws a recommendation (NIP-09), restricted to the address form.
const kindDeletion nostr.Kind = 5

// acceptedKinds is the single place to widen what the relay stores. NIP-42 AUTH (kind
// 22242) never reaches this list; khatru consumes it before OnEvent.
var acceptedKinds = map[nostr.Kind]struct{}{
	kindRecommendation: {},
	kindDeletion:       {},
}

const reasonNotAuthorOfDeletion = "blocked: only the author may delete an event"

func onlyAcceptedKinds(_ context.Context, event nostr.Event) (reject bool, msg string) {
	if _, ok := acceptedKinds[event.Kind]; !ok {
		return true, fmt.Sprintf("blocked: kind %d is not stored by this relay", event.Kind)
	}
	if event.Kind == kindDeletion {
		if _, ok := deletionAddress(event); !ok {
			return true, reasonNotAuthorOfDeletion
		}
	}
	return false, ""
}

// deletionAddress returns the one recommendation address a kind 5 names. ok is false
// unless the event carries exactly one `a` tag, of kind 32160, whose pubkey is the
// signer's own. `e` tags and content are ignored.
func deletionAddress(event nostr.Event) (a address, ok bool) {
	var tags []nostr.Tag
	for tag := range event.Tags.FindAll("a") {
		tags = append(tags, tag)
	}
	if len(tags) != 1 || len(tags[0]) < 2 {
		return a, false
	}
	a, ok = parseAddress(tags[0][1])
	if !ok || a.pubkey != event.PubKey {
		return a, false
	}
	return a, true
}
