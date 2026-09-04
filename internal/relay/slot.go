package relay

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/eventstore"
	"fiatjaf.com/nostr/eventstore/boltdb"
)

// address is <kind>:<pubkey>:<d> for a recommendation, the NIP-01 addressable form and
// the value of a deletion's `a` tag.
type address struct {
	pubkey nostr.PubKey
	d      string
}

func (a address) String() string {
	return fmt.Sprintf("%d:%s:%s", kindRecommendation, a.pubkey.Hex(), a.d)
}

// parseAddress reads a recommendation address; ok is false for any other kind or shape.
func parseAddress(s string) (a address, ok bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return a, false
	}
	kind, err := strconv.Atoi(parts[0])
	if err != nil || nostr.Kind(kind) != kindRecommendation {
		return a, false
	}
	pk, err := nostr.PubKeyFromHex(parts[1])
	if err != nil {
		return a, false
	}
	return address{pubkey: pk, d: parts[2]}, true
}

// slot enforces one record per address: a signer's recommendation or the deletion
// that withdrew it, never both. Every write reads the slot first, so writes are
// serialised by mu; reads through QueryEvents are unaffected.
type slot struct {
	mu    sync.Mutex
	store *boltdb.BoltBackend
}

// read returns what a holds: at most one recommendation and one deletion.
func (s *slot) read(a address) (rec, del *nostr.Event) {
	rec = s.one(nostr.Filter{
		Kinds: []nostr.Kind{kindRecommendation}, Authors: []nostr.PubKey{a.pubkey},
		Tags: nostr.TagMap{"d": []string{a.d}},
	})
	del = s.one(nostr.Filter{
		Kinds: []nostr.Kind{kindDeletion}, Authors: []nostr.PubKey{a.pubkey},
		Tags: nostr.TagMap{"a": []string{a.String()}},
	})
	return rec, del
}

func (s *slot) one(filter nostr.Filter) *nostr.Event {
	for evt := range s.store.QueryEvents(filter, 1) {
		return &evt
	}
	return nil
}

var errNewerDeletion = errors.New("blocked: a newer deletion exists for this address")

// storeRecommendation applies a kind 32160. A deletion created at or after the
// recommendation refuses it (contract rule 4). Against a stored recommendation the
// newer created_at wins and the stored one keeps a tie.
func (s *slot) storeRecommendation(evt nostr.Event) error {
	a := address{pubkey: evt.PubKey, d: evt.Tags.GetD()}
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, del := s.read(a)
	if del != nil && evt.CreatedAt <= del.CreatedAt {
		return errNewerDeletion
	}
	if rec != nil && evt.CreatedAt <= rec.CreatedAt {
		return eventstore.ErrDupEvent
	}
	return s.take(evt, rec, del)
}

// storeDeletion applies a validated kind 5. It removes a recommendation created at or
// before it (contract rule 2) and takes the slot; a newer recommendation or a newer
// deletion already in the slot leaves it unchanged.
func (s *slot) storeDeletion(evt nostr.Event) error {
	a, _ := deletionAddress(evt)
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, del := s.read(a)
	if rec != nil && evt.CreatedAt < rec.CreatedAt {
		return eventstore.ErrDupEvent
	}
	if del != nil && evt.CreatedAt <= del.CreatedAt {
		return eventstore.ErrDupEvent
	}
	return s.take(evt, rec, del)
}

// take removes whatever the slot holds and stores evt. ErrDupEvent from the callers
// makes khatru answer OK true without broadcasting: the relay does not hold the event.
func (s *slot) take(evt nostr.Event, held ...*nostr.Event) error {
	for _, old := range held {
		if old == nil {
			continue
		}
		if old.ID == evt.ID {
			return eventstore.ErrDupEvent
		}
		if err := s.store.DeleteEvent(old.ID); err != nil {
			return err
		}
	}
	return s.store.SaveEvent(evt)
}
