package relay_test

import (
	"strings"
	"testing"

	"fiatjaf.com/nostr"
)

func TestChallengesOnConnect(t *testing.T) {
	url := startRelay(t, nostr.Generate().Public())

	if challenge := dial(t, url).readChallenge(); challenge == "" {
		t.Fatal("empty challenge")
	}
}

func TestUnauthenticatedRequestIsClosedWithAuthRequired(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())

	_, closed := dial(t, url).request("feed", feedFilter(member.Public()))
	if !strings.HasPrefix(closed, "auth-required: ") {
		t.Errorf("CLOSED reason = %q, want auth-required: prefix", closed)
	}
}

func TestUnauthenticatedEventIsRefusedWithAuthRequired(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())

	ok, reason := dial(t, url).publish(recommendation(t, member, "tmdb:movie:1", nostr.Now()))
	if ok || !strings.HasPrefix(reason, "auth-required: ") {
		t.Errorf("OK = %v %q, want false with auth-required: prefix", ok, reason)
	}
}

func TestNonMemberAuthenticatesButIsRestricted(t *testing.T) {
	member, outsider := nostr.Generate(), nostr.Generate()
	url := startRelay(t, member.Public())

	c := connectAs(t, url, outsider) // AUTH itself succeeds; khatru cannot refuse it

	_, closed := c.request("feed", feedFilter(member.Public()))
	if !strings.HasPrefix(closed, "restricted: ") {
		t.Errorf("CLOSED reason = %q, want restricted: prefix", closed)
	}
	ok, reason := c.publish(recommendation(t, outsider, "tmdb:movie:1", nostr.Now()))
	if ok || !strings.HasPrefix(reason, "restricted: ") {
		t.Errorf("OK = %v %q, want false with restricted: prefix", ok, reason)
	}
}

func TestMemberCannotPublishAnOutsidersEvent(t *testing.T) {
	member, outsider := nostr.Generate(), nostr.Generate()
	url := startRelay(t, member.Public())

	ok, reason := connectAs(t, url, member).publish(recommendation(t, outsider, "tmdb:movie:1", nostr.Now()))
	if ok || !strings.HasPrefix(reason, "restricted: ") {
		t.Errorf("OK = %v %q, want false with restricted: prefix", ok, reason)
	}
}

func TestMemberReadsAnotherMembersRecommendation(t *testing.T) {
	alice, bob := nostr.Generate(), nostr.Generate()
	url := startRelay(t, alice.Public(), bob.Public())

	evt := recommendation(t, alice, "tmdb:movie:1", nostr.Now())
	mustPublish(t, connectAs(t, url, alice), evt)

	got := storedEvents(t, connectAs(t, url, bob), feedFilter(alice.Public(), bob.Public()))
	if len(got) != 1 || got[0].ID != evt.ID {
		t.Fatalf("got %d events, want alice's recommendation", len(got))
	}
}

func TestMemberReceivesLiveRecommendationFromAnotherMember(t *testing.T) {
	alice, bob := nostr.Generate(), nostr.Generate()
	url := startRelay(t, alice.Public(), bob.Public())

	reader := connectAs(t, url, bob)
	if _, closed := reader.request("feed", feedFilter(alice.Public(), bob.Public())); closed != "" {
		t.Fatalf("subscription closed: %s", closed)
	}

	evt := recommendation(t, alice, "tmdb:movie:1", nostr.Now())
	mustPublish(t, connectAs(t, url, alice), evt)

	if got := reader.readEvent("feed"); got.ID != evt.ID {
		t.Fatalf("got event %s, want %s", got.ID, evt.ID)
	}
}
