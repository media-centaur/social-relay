// Package relay assembles the khatru relay for a friend group: bbolt storage, the
// NIP-11 document, and the membership rules that gate every read and write.
package relay

import (
	"context"
	"fmt"
	"iter"
	"net/http"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/eventstore/boltdb"
	"fiatjaf.com/nostr/khatru"
)

// maxQueryLimit caps how many stored events one filter may return.
const maxQueryLimit = 1000

// Relay is an http.Handler serving the WebSocket endpoint and the NIP-11 document.
type Relay struct {
	khatru *khatru.Relay
	store  *boltdb.BoltBackend
}

// New opens the event store at cfg.Database and wires it into a khatru relay.
func New(cfg Config) (*Relay, error) {
	store := &boltdb.BoltBackend{Path: cfg.Database}
	if err := store.Init(); err != nil {
		return nil, fmt.Errorf("open event store %s: %w", cfg.Database, err)
	}

	rl := khatru.NewRelay()
	rl.Info.Name = cfg.Name
	rl.Info.Software = "https://github.com/media-centaur/social-relay"
	rl.Info.SupportedNIPs = []any{1, 11, 42}

	// Wired by hand rather than through UseEventstore: setting DeleteEvent or Count
	// makes khatru advertise NIP-9 and NIP-45, neither of which this relay offers.
	rl.QueryStored = func(ctx context.Context, filter nostr.Filter) iter.Seq[nostr.Event] {
		return store.QueryEvents(filter, maxQueryLimit)
	}
	rl.StoreEvent = func(ctx context.Context, event nostr.Event) error {
		return store.SaveEvent(event)
	}
	rl.ReplaceEvent = func(ctx context.Context, event nostr.Event) error {
		_, err := store.ReplaceEvent(event)
		return err
	}

	members := newMembership(cfg.Members)
	rl.OnConnect = members.challenge
	rl.OnRequest = members.onRequest
	rl.OnEvent = members.onEvent

	return &Relay{khatru: rl, store: store}, nil
}

func (r *Relay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.khatru.ServeHTTP(w, req)
}

// Close releases the event store. The HTTP server owning the handler is closed by its owner.
func (r *Relay) Close() {
	r.store.Close()
}
