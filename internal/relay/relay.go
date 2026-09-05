// Package relay assembles the khatru relay for a friend group: bbolt storage, the
// NIP-11 document, the membership rules that gate every read and write, and the
// NIP-86 management API through which admins change membership at runtime.
package relay

import (
	"context"
	"fmt"
	"iter"
	"net/http"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/eventstore/boltdb"
	"fiatjaf.com/nostr/khatru"
	"fiatjaf.com/nostr/khatru/policies"
)

// maxQueryLimit caps how many stored events one filter may return. The app pages in
// batches of 500 and treats a full batch as "ask again with until".
const maxQueryLimit = 500

// Relay is an http.Handler serving the WebSocket endpoint and the NIP-11 document.
type Relay struct {
	khatru *khatru.Relay
	store  *boltdb.BoltBackend
}

// New opens the event store at cfg.Database and wires it into a khatru relay.
// version is advertised in the NIP-11 document.
func New(version string, cfg Config) (*Relay, error) {
	store := &boltdb.BoltBackend{Path: cfg.Database}
	if err := store.Init(); err != nil {
		return nil, fmt.Errorf("open event store %s: %w", cfg.Database, err)
	}

	rl := khatru.NewRelay()
	// khatru checks AUTH relay tags against this URL and serves only its path.
	rl.ServiceURL = cfg.ServiceURL
	rl.Info.Name = cfg.Name
	rl.Info.Software = "https://github.com/media-centaur/social-relay"
	rl.Info.Version = version
	// NIP-9 is listed by hand: deletion is handled by the address slot below, not by
	// khatru's DeleteEvent path, which cannot keep one record per address (ADR-002).
	rl.Info.SupportedNIPs = []any{1, 9, 11, 42, 86}

	// Wired by hand rather than through UseEventstore: setting DeleteEvent or Count
	// makes khatru run its own NIP-9 handler and advertise NIP-45.
	rl.QueryStored = func(ctx context.Context, filter nostr.Filter) iter.Seq[nostr.Event] {
		return store.QueryEvents(filter, maxQueryLimit)
	}
	records := &slot{store: store}
	rl.StoreEvent = func(ctx context.Context, event nostr.Event) error {
		return records.storeDeletion(event)
	}
	rl.ReplaceEvent = func(ctx context.Context, event nostr.Event) error {
		return records.storeActivity(event)
	}

	members, err := newMembership(cfg.Admins, store.DB)
	if err != nil {
		store.Close()
		return nil, err
	}
	rl.OnConnect = members.challenge
	rl.OnRequest = members.onRequest
	rl.OnEvent = policies.SeqEvent(members.onEvent, onlyAcceptedKinds)
	rl.PreventBroadcast = members.preventBroadcast
	rl.OnAuth = members.logAuth(rl.Log)
	rl.ManagementAPI = managementAPI(members, rl.Log)

	return &Relay{khatru: rl, store: store}, nil
}

func (r *Relay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.khatru.ServeHTTP(w, req)
}

// Close releases the event store. The HTTP server owning the handler is closed by its owner.
func (r *Relay) Close() {
	r.store.Close()
}
