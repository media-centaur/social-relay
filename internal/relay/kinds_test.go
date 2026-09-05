package relay_test

import (
	"fmt"
	"strings"
	"testing"

	"fiatjaf.com/nostr"
)

func signedKind(t *testing.T, sk nostr.SecretKey, kind nostr.Kind) nostr.Event {
	t.Helper()
	evt := nostr.Event{Kind: kind, CreatedAt: nostr.Now(), Tags: nostr.Tags{{"d", "tmdb:movie:1"}}, Content: "placeholder"}
	if err := evt.Sign(sk); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return evt
}

func TestMemberCannotPublishOtherKinds(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())
	c := connectAs(t, url, member)

	for _, kind := range []nostr.Kind{1, 5, 30023} {
		ok, reason := c.publish(signedKind(t, member, kind))
		if ok || !strings.HasPrefix(reason, "blocked: ") {
			t.Errorf("kind %d: OK = %v %q, want false with blocked: prefix", kind, ok, reason)
		}
	}
}

func TestActivityKindsAreAccepted(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())
	c := connectAs(t, url, member)

	for _, kind := range []nostr.Kind{kindRecommendation, kindWatched, kindTracking} {
		if ok, reason := c.publish(signedKind(t, member, kind)); !ok {
			t.Errorf("kind %d refused: %s", kind, reason)
		}
	}
}

// One title, three kinds: three slots, each keeping its own record.
func TestKindsHoldSeparateSlotsForOneTitle(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())
	c := connectAs(t, url, member)

	for _, kind := range []nostr.Kind{kindRecommendation, kindWatched, kindTracking} {
		if ok, reason := c.publish(signedKind(t, member, kind)); !ok {
			t.Fatalf("kind %d refused: %s", kind, reason)
		}
	}

	got := storedEvents(t, connectAs(t, url, member), nostr.Filter{
		Kinds: []nostr.Kind{kindRecommendation, kindWatched, kindTracking}, Authors: []nostr.PubKey{member.Public()},
	})
	if len(got) != 3 {
		t.Fatalf("got %d events, want one per kind", len(got))
	}
}

// A deletion names the kind it withdraws; the other kinds' records stay.
func TestDeletionWithdrawsOnlyItsOwnKind(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())
	c := connectAs(t, url, member)

	for _, kind := range []nostr.Kind{kindRecommendation, kindWatched} {
		mustPublish(t, c, signedKind(t, member, kind))
	}
	addr := fmt.Sprintf("%d:%s:%s", kindWatched, member.Public().Hex(), "tmdb:movie:1")
	mustPublish(t, c, deletion(t, member, addr, nostr.Now()+1))

	got := storedEvents(t, connectAs(t, url, member), nostr.Filter{
		Kinds: []nostr.Kind{kindRecommendation, kindWatched}, Authors: []nostr.PubKey{member.Public()},
	})
	if len(got) != 1 || got[0].Kind != kindRecommendation {
		t.Fatalf("got %v, want only the recommendation left", got)
	}
}
