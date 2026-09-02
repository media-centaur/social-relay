package relay

import (
	"context"
	"fmt"

	"fiatjaf.com/nostr"
)

// kindRecommendation is the app's addressable recommendation event. The relay stores it
// and never interprets it; its shape is the app's contract (docs/protocol.md).
const kindRecommendation nostr.Kind = 32160

// acceptedKinds is the single place to widen what the relay stores. NIP-42 AUTH (kind
// 22242) never reaches this list; khatru consumes it before OnEvent.
var acceptedKinds = map[nostr.Kind]struct{}{
	kindRecommendation: {},
}

func onlyAcceptedKinds(_ context.Context, event nostr.Event) (reject bool, msg string) {
	if _, ok := acceptedKinds[event.Kind]; ok {
		return false, ""
	}
	return true, fmt.Sprintf("blocked: kind %d is not stored by this relay", event.Kind)
}
