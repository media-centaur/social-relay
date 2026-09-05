package relay

import (
	"context"
	"fmt"

	"fiatjaf.com/nostr"
)

// The app's addressable activity kinds. The relay stores them and never interprets
// them; their shape is the app's contract (docs/protocol.md). Every one shares the
// address slot rules: one record per signer per kind per `d` tag.
const (
	kindRecommendation nostr.Kind = 32160
	kindWatched        nostr.Kind = 32161
	kindTracking       nostr.Kind = 32162
)

// kindDeletion withdraws an activity of any kind (NIP-09), restricted to the address
// form.
const kindDeletion nostr.Kind = 5

// activityKinds is the single place to widen what the relay stores as an activity.
var activityKinds = map[nostr.Kind]struct{}{
	kindRecommendation: {},
	kindWatched:        {},
	kindTracking:       {},
}

// acceptedKinds is everything the relay stores. NIP-42 AUTH (kind 22242) never
// reaches this list; khatru consumes it before OnEvent.
var acceptedKinds = func() map[nostr.Kind]struct{} {
	m := map[nostr.Kind]struct{}{kindDeletion: {}}
	for k := range activityKinds {
		m[k] = struct{}{}
	}
	return m
}()

func isActivityKind(k nostr.Kind) bool {
	_, ok := activityKinds[k]
	return ok
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

// deletionAddress returns the one activity address a kind 5 names. ok is false
// unless the event carries exactly one `a` tag, of an activity kind, whose pubkey is
// the signer's own. `e` tags and content are ignored.
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
