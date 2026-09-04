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
service_url = "wss://relay.example"
admins = ["`+nip19.EncodeNpub(alice)+`", "`+nip19.EncodeNpub(bob)+`"]
`)
	cfg, err := relay.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := relay.Config{
		Name:       "our relay",
		Listen:     "127.0.0.1:2170",
		Database:   "/data/events.db",
		ServiceURL: "wss://relay.example",
		Admins:     []nostr.PubKey{alice, bob},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadConfigRequiresAtLeastOneAdmin(t *testing.T) {
	path := writeConfig(t, `
listen = "127.0.0.1:2170"
database = "/data/events.db"
service_url = "wss://relay.example"
admins = []
`)
	if _, err := relay.LoadConfig(path); err == nil || !strings.Contains(err.Error(), "admins") {
		t.Fatalf("err = %v, want an error naming admins", err)
	}
}

func TestLoadConfigRejectsMalformedAdmin(t *testing.T) {
	path := writeConfig(t, `
listen = "127.0.0.1:2170"
database = "/data/events.db"
service_url = "wss://relay.example"
admins = ["`+nostr.Generate().Public().Hex()+`"]
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
service_url = "wss://relay.example"
admins = ["`+nip19.EncodeNpub(nostr.Generate().Public())+`"]
member = ["npub1placeholder"]
`)
	if _, err := relay.LoadConfig(path); err == nil {
		t.Fatal("expected an error for the misspelled key, got nil")
	}
}

func TestLoadConfigRequiresListenDatabaseAndServiceURL(t *testing.T) {
	path := writeConfig(t, `name = "our relay"`)
	_, err := relay.LoadConfig(path)
	if err == nil {
		t.Fatal("expected an error for missing keys, got nil")
	}
	for _, key := range []string{"listen", "database", "service_url"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name missing key %s", err, key)
		}
	}
}

func TestLoadConfigRejectsServiceURLThatIsNotWebSocket(t *testing.T) {
	path := writeConfig(t, `
listen = "127.0.0.1:2170"
database = "/data/events.db"
service_url = "https://relay.example"
admins = ["`+nip19.EncodeNpub(nostr.Generate().Public())+`"]
`)
	if _, err := relay.LoadConfig(path); err == nil || !strings.Contains(err.Error(), "service_url") {
		t.Fatalf("err = %v, want an error naming service_url", err)
	}
}
