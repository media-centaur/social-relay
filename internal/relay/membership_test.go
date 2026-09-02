package relay_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"fiatjaf.com/nostr"
)

// closedReason issues a REQ and returns the CLOSED reason, failing if the relay
// answers with events or EOSE instead.
func closedReason(t *testing.T, c *nostr.Relay, filter nostr.Filter) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub, err := c.Subscribe(ctx, filter, nostr.SubscriptionOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	select {
	case reason := <-sub.ClosedReason:
		return reason
	case evt := <-sub.Events:
		t.Fatalf("relay returned event %s instead of CLOSED", evt.ID)
	case <-sub.EndOfStoredEvents:
		t.Fatal("relay answered EOSE instead of CLOSED")
	case <-ctx.Done():
		t.Fatal("no CLOSED within timeout")
	}
	return ""
}

func publishError(t *testing.T, c *nostr.Relay, evt nostr.Event) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.Publish(ctx, evt)
	if err == nil {
		t.Fatal("publish succeeded, want OK false")
	}
	return err.Error()
}

func feedFilter(authors ...nostr.PubKey) nostr.Filter {
	return nostr.Filter{Kinds: []nostr.Kind{kindRecommendation}, Authors: authors}
}

func TestChallengesOnConnect(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())

	// authenticate fails fast with "no challenge" if the relay never sent AUTH;
	// success proves a challenge arrived without the client asking for one.
	authenticate(t, connect(t, url), member)
}

func TestUnauthenticatedRequestIsClosedWithAuthRequired(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())

	reason := closedReason(t, connect(t, url), feedFilter(member.Public()))
	if !strings.HasPrefix(reason, "auth-required: ") {
		t.Errorf("CLOSED reason = %q, want auth-required: prefix", reason)
	}
}

func TestUnauthenticatedEventIsRefusedWithAuthRequired(t *testing.T) {
	member := nostr.Generate()
	url := startRelay(t, member.Public())

	reason := publishError(t, connect(t, url), recommendation(t, member, "tmdb:movie:1", nostr.Now()))
	if !strings.Contains(reason, "auth-required: ") {
		t.Errorf("OK reason = %q, want auth-required: prefix", reason)
	}
}

func TestNonMemberAuthenticatesButIsRestricted(t *testing.T) {
	member, outsider := nostr.Generate(), nostr.Generate()
	url := startRelay(t, member.Public())

	c := connectAs(t, url, outsider) // AUTH itself succeeds; khatru cannot refuse it

	reason := closedReason(t, c, feedFilter(member.Public()))
	if !strings.HasPrefix(reason, "restricted: ") {
		t.Errorf("CLOSED reason = %q, want restricted: prefix", reason)
	}
	reason = publishError(t, c, recommendation(t, outsider, "tmdb:movie:1", nostr.Now()))
	if !strings.Contains(reason, "restricted: ") {
		t.Errorf("OK reason = %q, want restricted: prefix", reason)
	}
}

func TestMemberCannotPublishAnOutsidersEvent(t *testing.T) {
	member, outsider := nostr.Generate(), nostr.Generate()
	url := startRelay(t, member.Public())

	c := connectAs(t, url, member)
	reason := publishError(t, c, recommendation(t, outsider, "tmdb:movie:1", nostr.Now()))
	if !strings.Contains(reason, "restricted: ") {
		t.Errorf("OK reason = %q, want restricted: prefix", reason)
	}
}

func TestMemberReadsAnotherMembersRecommendation(t *testing.T) {
	alice, bob := nostr.Generate(), nostr.Generate()
	url := startRelay(t, alice.Public(), bob.Public())

	evt := recommendation(t, alice, "tmdb:movie:1", nostr.Now())
	publish(t, connectAs(t, url, alice), evt)

	got := storedEvents(t, connectAs(t, url, bob), feedFilter(alice.Public(), bob.Public()))
	if len(got) != 1 || got[0].ID != evt.ID {
		t.Fatalf("got %d events, want alice's recommendation", len(got))
	}
}

func TestMemberReceivesLiveRecommendationFromAnotherMember(t *testing.T) {
	alice, bob := nostr.Generate(), nostr.Generate()
	url := startRelay(t, alice.Public(), bob.Public())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub, err := connectAs(t, url, bob).Subscribe(ctx, feedFilter(alice.Public(), bob.Public()), nostr.SubscriptionOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	<-sub.EndOfStoredEvents

	evt := recommendation(t, alice, "tmdb:movie:1", nostr.Now())
	publish(t, connectAs(t, url, alice), evt)

	select {
	case got := <-sub.Events:
		if got.ID != evt.ID {
			t.Fatalf("got event %s, want %s", got.ID, evt.ID)
		}
	case reason := <-sub.ClosedReason:
		t.Fatalf("subscription closed: %s", reason)
	case <-ctx.Done():
		t.Fatal("live event not delivered within timeout")
	}
}
