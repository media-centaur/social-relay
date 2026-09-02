package relay

import (
	"context"
	"fmt"
	"log"
	"sync"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/khatru"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip86"
	"go.etcd.io/bbolt"
)

// Rejection reasons. These strings are part of the client contract in docs/protocol.md;
// the app matches on the NIP-01 prefix before the colon.
const (
	reasonAuthRequiredRead  = "auth-required: authenticate to read from this relay"
	reasonAuthRequiredWrite = "auth-required: authenticate to write to this relay"
	reasonNotMember         = "restricted: this key is not a member of this relay"
	reasonAuthorNotMember   = "restricted: the event author is not a member of this relay"
	reasonNotAdmin          = "restricted: only admins may manage this relay"
)

// membersBucket holds allowed keys in the relay's bbolt file: key = pubkey bytes,
// value = the reason given when allowing. The eventstore ignores buckets it does not own.
var membersBucket = []byte("socialRelayMembers")

// membership decides who may read and write: admins from the config, plus keys
// allowed at runtime and persisted in the database.
type membership struct {
	admins map[nostr.PubKey]struct{}
	db     *bbolt.DB

	mu      sync.RWMutex
	allowed map[nostr.PubKey]string // pubkey -> reason
}

func newMembership(admins []nostr.PubKey, db *bbolt.DB) (*membership, error) {
	m := &membership{
		admins:  make(map[nostr.PubKey]struct{}, len(admins)),
		db:      db,
		allowed: make(map[nostr.PubKey]string),
	}
	for _, pk := range admins {
		m.admins[pk] = struct{}{}
	}
	err := db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(membersBucket)
		if err != nil {
			return err
		}
		return b.ForEach(func(k, v []byte) error {
			var pk nostr.PubKey
			if len(k) != len(pk) {
				return fmt.Errorf("corrupt member key of %d bytes", len(k))
			}
			copy(pk[:], k)
			m.allowed[pk] = string(v)
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("load members: %w", err)
	}
	return m, nil
}

func (m *membership) isAdmin(pk nostr.PubKey) bool {
	_, ok := m.admins[pk]
	return ok
}

func (m *membership) isMember(pk nostr.PubKey) bool {
	if m.isAdmin(pk) {
		return true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.allowed[pk]
	return ok
}

// allow adds pk. Allowing an admin or an existing member is a no-op.
func (m *membership) allow(pk nostr.PubKey, reason string) error {
	if m.isAdmin(pk) {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(membersBucket).Put(pk[:], []byte(reason))
	}); err != nil {
		return fmt.Errorf("store member: %w", err)
	}
	m.allowed[pk] = reason
	return nil
}

// unallow removes pk. Admins cannot be removed here; they are set in the config file.
func (m *membership) unallow(pk nostr.PubKey) error {
	if m.isAdmin(pk) {
		return fmt.Errorf("%s is an admin; admins are set in the config file", nip19.EncodeNpub(pk))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(membersBucket).Delete(pk[:])
	}); err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	delete(m.allowed, pk)
	return nil
}

// list returns admins first, then allowed keys.
func (m *membership) list() []nip86.PubKeyReason {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]nip86.PubKeyReason, 0, len(m.admins)+len(m.allowed))
	for pk := range m.admins {
		out = append(out, nip86.PubKeyReason{PubKey: pk, Reason: "admin"})
	}
	for pk, reason := range m.allowed {
		out = append(out, nip86.PubKeyReason{PubKey: pk, Reason: reason})
	}
	return out
}

// challenge sends the NIP-42 AUTH challenge as soon as the socket opens. The app
// answers challenges proactively and never reacts to auth-required: rejections.
func (m *membership) challenge(ctx context.Context) {
	khatru.RequestAuth(ctx)
}

func (m *membership) onRequest(ctx context.Context, _ nostr.Filter) (reject bool, msg string) {
	pk, authed := khatru.GetAuthed(ctx)
	switch {
	case !authed:
		return true, reasonAuthRequiredRead
	case !m.isMember(pk):
		return true, reasonNotMember
	}
	return false, ""
}

func (m *membership) onEvent(ctx context.Context, event nostr.Event) (reject bool, msg string) {
	pk, authed := khatru.GetAuthed(ctx)
	switch {
	case !authed:
		return true, reasonAuthRequiredWrite
	case !m.isMember(pk):
		return true, reasonNotMember
	case !m.isMember(event.PubKey):
		return true, reasonAuthorNotMember
	}
	return false, ""
}

// preventBroadcast stops live delivery to a socket whose key has been removed since
// it subscribed. Stored-event reads are already gated per REQ.
func (m *membership) preventBroadcast(ws *khatru.WebSocket, _ nostr.Filter, _ nostr.Event) bool {
	keys := ws.AuthedPublicKeys
	if len(keys) == 0 {
		return true
	}
	return !m.isMember(keys[len(keys)-1])
}

// logAuth records every successful AUTH so an operator can see who is knocking.
// khatru accepts any well-formed AUTH; membership is decided on REQ and EVENT.
func (m *membership) logAuth(logger *log.Logger) func(context.Context, nostr.PubKey) {
	return func(_ context.Context, pk nostr.PubKey) {
		status := "not a member"
		switch {
		case m.isAdmin(pk):
			status = "admin"
		case m.isMember(pk):
			status = "member"
		}
		logger.Printf("authenticated %s (%s)", nip19.EncodeNpub(pk), status)
	}
}
