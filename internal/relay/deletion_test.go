package relay_test

import (
	"fmt"
	"testing"

	"fiatjaf.com/nostr"
)

const kindDeletion = nostr.Kind(5)

func address(pk nostr.PubKey, d string) string {
	return fmt.Sprintf("%d:%s:%s", kindRecommendation, pk.Hex(), d)
}

// deletion signs a kind 5 naming addr, the NIP-09 address form the app publishes.
func deletion(t *testing.T, sk nostr.SecretKey, addr string, createdAt nostr.Timestamp) nostr.Event {
	t.Helper()
	evt := nostr.Event{Kind: kindDeletion, CreatedAt: createdAt, Tags: nostr.Tags{{"a", addr}}}
	if err := evt.Sign(sk); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return evt
}

func deletionsOf(pk nostr.PubKey) nostr.Filter {
	return nostr.Filter{Kinds: []nostr.Kind{kindDeletion}, Authors: []nostr.PubKey{pk}}
}

func TestAuthorDeletesOwnRecommendation(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)

	mustPublish(t, c, recommendation(t, sk, "tmdb:movie:1", nostr.Now()-10))
	del := deletion(t, sk, address(sk.Public(), "tmdb:movie:1"), nostr.Now())
	mustPublish(t, c, del)

	if got := storedEvents(t, c, feedFilter(sk.Public())); len(got) != 0 {
		t.Errorf("recommendation still stored after deletion: %v", got)
	}
	got := storedEvents(t, c, deletionsOf(sk.Public()))
	if len(got) != 1 || got[0].ID != del.ID {
		t.Errorf("deletions = %v, want exactly the published deletion", got)
	}
}

func TestDeletionMustNameTheSignersOwnRecommendation(t *testing.T) {
	sk, other := nostr.Generate(), nostr.Generate()
	url := startRelay(t, sk.Public(), other.Public())
	c := connectAs(t, url, sk)

	cases := map[string]nostr.Tags{
		"another signer's address": {{"a", address(other.Public(), "tmdb:movie:1")}},
		"no a tag":                 {{"e", "0000000000000000000000000000000000000000000000000000000000000000"}},
		"two a tags":               {{"a", address(sk.Public(), "tmdb:movie:1")}, {"a", address(sk.Public(), "tmdb:movie:2")}},
		"another kind":             {{"a", "30023:" + sk.Public().Hex() + ":post"}},
		"malformed address":        {{"a", "32160"}},
	}
	for name, tags := range cases {
		evt := nostr.Event{Kind: kindDeletion, CreatedAt: nostr.Now(), Tags: tags}
		if err := evt.Sign(sk); err != nil {
			t.Fatalf("sign: %v", err)
		}
		ok, reason := c.publish(evt)
		if ok || reason != "blocked: only the author may delete an event" {
			t.Errorf("%s: OK = %v %q, want false with the author-only reason", name, ok, reason)
		}
	}
}

func TestRecommendationOlderThanStoredDeletionIsRefused(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)

	mustPublish(t, c, deletion(t, sk, address(sk.Public(), "tmdb:movie:1"), nostr.Now()))
	ok, reason := c.publish(recommendation(t, sk, "tmdb:movie:1", nostr.Now()-10))
	if ok || reason != "blocked: a newer deletion exists for this address" {
		t.Errorf("OK = %v %q, want false with the newer-deletion reason", ok, reason)
	}
	if got := storedEvents(t, c, feedFilter(sk.Public())); len(got) != 0 {
		t.Errorf("refused recommendation was stored: %v", got)
	}
}

func TestNewerRecommendationReplacesStoredDeletion(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)

	mustPublish(t, c, deletion(t, sk, address(sk.Public(), "tmdb:movie:1"), nostr.Now()-10))
	rec := recommendation(t, sk, "tmdb:movie:1", nostr.Now())
	mustPublish(t, c, rec)

	if got := storedEvents(t, c, deletionsOf(sk.Public())); len(got) != 0 {
		t.Errorf("deletion still stored after a newer recommendation: %v", got)
	}
	got := storedEvents(t, c, feedFilter(sk.Public()))
	if len(got) != 1 || got[0].ID != rec.ID {
		t.Errorf("recommendations = %v, want exactly the new one", got)
	}
}

func TestDeletionOlderThanStoredRecommendationIsDiscarded(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)

	rec := recommendation(t, sk, "tmdb:movie:1", nostr.Now())
	mustPublish(t, c, rec)
	mustPublish(t, c, deletion(t, sk, address(sk.Public(), "tmdb:movie:1"), nostr.Now()-10))

	got := storedEvents(t, c, feedFilter(sk.Public()))
	if len(got) != 1 || got[0].ID != rec.ID {
		t.Errorf("recommendations = %v, want the newer recommendation kept", got)
	}
	if got := storedEvents(t, c, deletionsOf(sk.Public())); len(got) != 0 {
		t.Errorf("stale deletion was stored: %v", got)
	}
}

func TestAddressHoldsOneDeletion(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)
	addr := address(sk.Public(), "tmdb:movie:1")

	newer := deletion(t, sk, addr, nostr.Now())
	mustPublish(t, c, deletion(t, sk, addr, nostr.Now()-20))
	mustPublish(t, c, newer)
	mustPublish(t, c, deletion(t, sk, addr, nostr.Now()-10))

	got := storedEvents(t, c, deletionsOf(sk.Public()))
	if len(got) != 1 || got[0].ID != newer.ID {
		t.Errorf("deletions = %v, want only the newest", got)
	}
}

func TestStoredRecommendationWinsATie(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)
	at := nostr.Now()

	first := recommendation(t, sk, "tmdb:movie:1", at)
	mustPublish(t, c, first)
	// Same address and created_at, different content so the id differs.
	second := nostr.Event{Kind: kindRecommendation, CreatedAt: at, Tags: nostr.Tags{{"d", "tmdb:movie:1"}}, Content: `{"title":"Other Placeholder"}`}
	if err := second.Sign(sk); err != nil {
		t.Fatalf("sign: %v", err)
	}
	mustPublish(t, c, second)

	got := storedEvents(t, c, feedFilter(sk.Public()))
	if len(got) != 1 || got[0].ID != first.ID {
		t.Errorf("recommendations = %v, want the first one kept", got)
	}
}

func TestDeletionForAnUnknownAddressIsStored(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)

	del := deletion(t, sk, address(sk.Public(), "tmdb:movie:1"), nostr.Now())
	mustPublish(t, c, del)

	got := storedEvents(t, c, deletionsOf(sk.Public()))
	if len(got) != 1 || got[0].ID != del.ID {
		t.Errorf("deletions = %v, want the deletion kept as a tombstone", got)
	}
}

func TestNonMemberCannotReadDeletions(t *testing.T) {
	member, stranger := nostr.Generate(), nostr.Generate()
	url := startRelay(t, member.Public())

	_, closed := connectAs(t, url, stranger).request("feed", deletionsOf(member.Public()))
	if closed != "restricted: this key is not a member of this relay" {
		t.Errorf("CLOSED reason = %q, want restricted:", closed)
	}
}

func TestRequestLimitIsCappedAt500(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)

	for i := range 520 {
		mustPublish(t, c, recommendation(t, sk, fmt.Sprintf("tmdb:movie:%d", i), nostr.Now()))
	}

	filter := feedFilter(sk.Public())
	filter.Limit = 1000
	if got := storedEvents(t, c, filter); len(got) != 500 {
		t.Errorf("got %d events for limit 1000, want 500", len(got))
	}
}
