package relay_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func connect(t *testing.T, url string) *nostr.Relay {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := nostr.RelayConnect(ctx, url, nostr.RelayOptions{})
	if err != nil {
		t.Fatalf("connect %s: %v", url, err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// authenticate answers the relay's NIP-42 challenge with sk. The challenge is sent on
// connect, so the first attempts may race it; retry until the client has one.
func authenticate(t *testing.T, c *nostr.Relay, sk nostr.SecretKey) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := c.Auth(ctx, func(_ context.Context, evt *nostr.Event) error { return evt.Sign(sk) })
		cancel()
		if err == nil {
			return
		}
		if !strings.Contains(err.Error(), "no challenge") || time.Now().After(deadline) {
			t.Fatalf("auth: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// connectAs opens a connection and authenticates it as sk.
func connectAs(t *testing.T, url string, sk nostr.SecretKey) *nostr.Relay {
	t.Helper()
	c := connect(t, url)
	authenticate(t, c, sk)
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

func publish(t *testing.T, c *nostr.Relay, evt nostr.Event) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Publish(ctx, evt); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// storedEvents issues a REQ and collects everything the relay returns before EOSE.
func storedEvents(t *testing.T, c *nostr.Relay, filter nostr.Filter) []nostr.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub, err := c.Subscribe(ctx, filter, nostr.SubscriptionOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	var got []nostr.Event
	for {
		select {
		case evt := <-sub.Events:
			got = append(got, evt)
		case <-sub.EndOfStoredEvents:
			return got
		case reason := <-sub.ClosedReason:
			t.Fatalf("subscription closed: %s", reason)
		case <-ctx.Done():
			t.Fatalf("no EOSE within timeout; got %d events", len(got))
		}
	}
}

func TestRelayInformationDocumentNamesRelayAndSupportedNIPs(t *testing.T) {
	url := startRelay(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	c := connectAs(t, url, sk)

	evt := recommendation(t, sk, "tmdb:movie:1", nostr.Now())
	publish(t, c, evt)

	got := storedEvents(t, connectAs(t, url, sk), nostr.Filter{
		Kinds:   []nostr.Kind{kindRecommendation},
		Authors: []nostr.PubKey{sk.Public()},
	})
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
	publish(t, c, older)
	publish(t, c, newer)

	got := storedEvents(t, connectAs(t, url, sk), nostr.Filter{
		Kinds:   []nostr.Kind{kindRecommendation},
		Authors: []nostr.PubKey{sk.Public()},
	})
	if len(got) != 1 || got[0].ID != newer.ID {
		t.Fatalf("got %d events %v, want only the newer one", len(got), got)
	}
}

func TestEmptyStoreAnswersWithEOSEOnly(t *testing.T) {
	sk := nostr.Generate()
	url := startRelay(t, sk.Public())

	got := storedEvents(t, connectAs(t, url, sk), nostr.Filter{
		Kinds:   []nostr.Kind{kindRecommendation},
		Authors: []nostr.PubKey{sk.Public()},
	})
	if len(got) != 0 {
		t.Fatalf("got %d events from an empty store", len(got))
	}
}
