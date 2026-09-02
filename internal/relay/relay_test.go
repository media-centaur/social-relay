package relay_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip11"

	"github.com/media-centaur/social-relay/internal/relay"
)

const kindRecommendation = nostr.Kind(32160)

// startRelay runs an in-process relay on a loopback port and returns its ws:// URL.
func startRelay(t *testing.T, members ...nostr.PubKey) string {
	t.Helper()
	r, err := relay.New(relay.Config{
		Name:     "test relay",
		Database: filepath.Join(t.TempDir(), "events.db"),
		Members:  members,
	})
	if err != nil {
		t.Fatalf("relay.New: %v", err)
	}
	t.Cleanup(r.Close)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// connectAs dials the relay and authenticates as sk, failing the test if refused.
func connectAs(t *testing.T, url string, sk nostr.SecretKey) *client {
	t.Helper()
	c := dial(t, url)
	if ok, reason := c.auth(sk); !ok {
		t.Fatalf("auth refused: %s", reason)
	}
	return c
}

func recommendation(t *testing.T, sk nostr.SecretKey, d string, createdAt nostr.Timestamp) nostr.Event {
	t.Helper()
	evt := nostr.Event{
		Kind:      kindRecommendation,
		CreatedAt: createdAt,
		Tags:      nostr.Tags{{"d", d}},
		Content:   `{"title":"Placeholder Title"}`,
	}
	if err := evt.Sign(sk); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return evt
}

func mustPublish(t *testing.T, c *client, evt nostr.Event) {
	t.Helper()
	if ok, reason := c.publish(evt); !ok {
		t.Fatalf("publish refused: %s", reason)
	}
}

// storedEvents issues a REQ and returns what the relay holds, failing on CLOSED.
func storedEvents(t *testing.T, c *client, filter nostr.Filter) []nostr.Event {
	t.Helper()
	events, closed := c.request("stored", filter)
	if closed != "" {
		t.Fatalf("subscription closed: %s", closed)
	}
	return events
}

func feedFilter(authors ...nostr.PubKey) nostr.Filter {
	return nostr.Filter{Kinds: []nostr.Kind{kindRecommendation}, Authors: authors}
}

func TestRelayInformationDocumentNamesRelayAndSupportedNIPs(t *testing.T) {
	url := startRelay(t, nostr.Generate().Public())

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	info, err := nip11.Fetch(ctx, url)
	if err != nil {
		t.Fatalf("nip11.Fetch: %v", err)
	}

	if info.Name != "test relay" {
		t.Errorf("name = %q, want %q", info.Name, "test relay")
	}
	got := fmt.Sprint(info.SupportedNIPs)
	if got != "[1 11 42]" {
		t.Errorf("supported_nips = %s, want [1 11 42]", got)
	}
}

func TestStoresRecommendationAndReturnsItBeforeEOSE(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())

	evt := recommendation(t, sk, "tmdb:movie:1", nostr.Now())
	mustPublish(t, connectAs(t, url, sk), evt)

	got := storedEvents(t, connectAs(t, url, sk), feedFilter(sk.Public()))
	if len(got) != 1 || got[0].ID != evt.ID {
		t.Fatalf("got %d events %v, want exactly the published one", len(got), got)
	}
}

func TestNewerEventWithSameAddressReplacesOlder(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())
	c := connectAs(t, url, sk)

	older := recommendation(t, sk, "tmdb:tv:2", nostr.Now()-10)
	newer := recommendation(t, sk, "tmdb:tv:2", nostr.Now())
	mustPublish(t, c, older)
	mustPublish(t, c, newer)

	got := storedEvents(t, connectAs(t, url, sk), feedFilter(sk.Public()))
	if len(got) != 1 || got[0].ID != newer.ID {
		t.Fatalf("got %d events %v, want only the newer one", len(got), got)
	}
}

func TestEmptyStoreAnswersWithEOSEOnly(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())

	got := storedEvents(t, connectAs(t, url, sk), feedFilter(sk.Public()))
	if len(got) != 0 {
		t.Fatalf("got %d events from an empty store", len(got))
	}
}

