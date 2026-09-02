package relay_test

import (
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

func TestRecommendationKindIsAccepted(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())

	if ok, reason := connectAs(t, url, member).publish(signedKind(t, member, kindRecommendation)); !ok {
		t.Fatalf("kind 32160 refused: %s", reason)
	}
}
