package relay_test

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fiatjaf.com/nostr"

	"github.com/media-centaur/social-relay/internal/relay"
)

// startRelayAt runs a relay that believes its public address is serviceURL while
// actually listening on loopback, the way it sits behind an operator's reverse proxy.
func startRelayAt(t *testing.T, serviceURL string, member nostr.PubKey) string {
	t.Helper()
	r, err := relay.New(testVersion, relay.Config{
		Name:       "test relay",
		Database:   filepath.Join(t.TempDir(), "events.db"),
		Admins:     []nostr.PubKey{member},
		ServiceURL: serviceURL,
	})
	if err != nil {
		t.Fatalf("relay.New: %v", err)
	}
	t.Cleanup(r.Close)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestAuthAcceptsRelayTagMatchingServiceURL(t *testing.T) {
	member := nostr.Generate()
	url := startRelayAt(t, "wss://relay.example", member.Public())

	for _, tag := range []string{
		"wss://relay.example",
		"wss://relay.example/",
		"WSS://Relay.Example",
	} {
		if ok, reason := dial(t, url).authAs(member, tag); !ok {
			t.Errorf("relay tag %q refused: %s", tag, reason)
		}
	}
}

func TestAuthRefusesRelayTagNotMatchingServiceURL(t *testing.T) {
	member := nostr.Generate()
	url := startRelayAt(t, "wss://relay.example", member.Public())

	for _, tag := range []string{
		"ws://relay.example",
		"wss://other.example",
		"wss://relay.example/relay",
		url, // the loopback address the test actually dials
	} {
		if ok, _ := dial(t, url).authAs(member, tag); ok {
			t.Errorf("relay tag %q accepted, want refused", tag)
		}
	}
}

func TestServiceURLWithPathServesOnlyThatPath(t *testing.T) {
	member := nostr.Generate()
	url := startRelayAt(t, "wss://home.example/relay", member.Public())

	if ok, reason := dial(t, url+"/relay").authAs(member, "wss://home.example/relay"); !ok {
		t.Errorf("auth on the configured path refused: %s", reason)
	}
}
