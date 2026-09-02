package relay_test

import (
	"os"
	"path/filepath"
	"testing"

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

func TestLoadConfigReadsNameListenAndDatabase(t *testing.T) {
	path := writeConfig(t, `
name = "our relay"
listen = "127.0.0.1:2170"
database = "/data/events.db"
`)
	cfg, err := relay.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := relay.Config{Name: "our relay", Listen: "127.0.0.1:2170", Database: "/data/events.db"}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, `
name = "our relay"
listen = "127.0.0.1:2170"
database = "/data/events.db"
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
