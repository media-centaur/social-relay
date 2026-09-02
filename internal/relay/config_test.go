package relay_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"

	"github.com/media-centaur/social-relay/internal/relay"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "relay.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigReadsEveryKey(t *testing.T) {
	alice, bob := nostr.Generate().Public(), nostr.Generate().Public()
	path := writeConfig(t, `
name = "our relay"
listen = "127.0.0.1:2170"
database = "/data/events.db"
members = ["`+nip19.EncodeNpub(alice)+`", "`+nip19.EncodeNpub(bob)+`"]
`)
	cfg, err := relay.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := relay.Config{
		Name:     "our relay",
		Listen:   "127.0.0.1:2170",
		Database: "/data/events.db",
		Members:  []nostr.PubKey{alice, bob},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadConfigRequiresAtLeastOneMember(t *testing.T) {
	path := writeConfig(t, `
listen = "127.0.0.1:2170"
database = "/data/events.db"
members = []
`)
	if _, err := relay.LoadConfig(path); err == nil || !strings.Contains(err.Error(), "members") {
		t.Fatalf("err = %v, want an error naming members", err)
	}
}

func TestLoadConfigRejectsMalformedMember(t *testing.T) {
	path := writeConfig(t, `
listen = "127.0.0.1:2170"
database = "/data/events.db"
members = ["`+nostr.Generate().Public().Hex()+`"]
`)
	if _, err := relay.LoadConfig(path); err == nil || !strings.Contains(err.Error(), "npub") {
		t.Fatalf("err = %v, want an error explaining npub is required", err)
	}
}

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, `
name = "our relay"
listen = "127.0.0.1:2170"
database = "/data/events.db"
members = ["`+nip19.EncodeNpub(nostr.Generate().Public())+`"]
member = ["npub1placeholder"]
`)
	if _, err := relay.LoadConfig(path); err == nil {
		t.Fatal("expected an error for the misspelled key, got nil")
	}
}

func TestLoadConfigRequiresListenAndDatabase(t *testing.T) {
	path := writeConfig(t, `name = "our relay"`)
	if _, err := relay.LoadConfig(path); err == nil {
		t.Fatal("expected an error for missing listen and database, got nil")
	}
}
