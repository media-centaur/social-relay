package relay

import (
	"context"
	"log"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/khatru"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/nip86"
)

// managementAPI exposes membership over NIP-86. Every method requires an admin;
// khatru has already verified the NIP-98 signature and URL by the time these run.
func managementAPI(m *membership, logger *log.Logger) khatru.RelayManagementAPI {
	return khatru.RelayManagementAPI{
		OnAPICall: func(ctx context.Context, _ nip86.MethodParams) (reject bool, msg string) {
			pk, ok := khatru.GetAuthed(ctx)
			if !ok || !m.isAdmin(pk) {
				return true, reasonNotAdmin
			}
			return false, ""
		},
		AllowPubKey: func(ctx context.Context, pk nostr.PubKey, reason string) error {
			if err := m.allow(pk, reason); err != nil {
				return err
			}
			logger.Printf("allowed %s (%s)", nip19.EncodeNpub(pk), reason)
			return nil
		},
		UnallowPubKey: func(ctx context.Context, pk nostr.PubKey, reason string) error {
			if err := m.unallow(pk); err != nil {
				return err
			}
			logger.Printf("unallowed %s (%s)", nip19.EncodeNpub(pk), reason)
			return nil
		},
		ListAllowedPubKeys: func(ctx context.Context) ([]nip86.PubKeyReason, error) {
			return m.list(), nil
		},
	}
}
