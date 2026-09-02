package relay

import (
	"fmt"
	"os"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"github.com/BurntSushi/toml"
)

// Config is the operator's TOML file, decoded and validated.
type Config struct {
	// Name is advertised in the NIP-11 relay information document.
	Name string
	// Listen is the plain-HTTP address the relay binds; TLS is the reverse proxy's job.
	Listen string
	// Database is the path of the bbolt file holding every stored event.
	Database string
	// Members are the public keys allowed to read and write. Nobody else gets either.
	Members []nostr.PubKey
}

// fileConfig mirrors the TOML file before validation. Members are npub strings there.
type fileConfig struct {
	Name     string   `toml:"name"`
	Listen   string   `toml:"listen"`
	Database string   `toml:"database"`
	Members  []string `toml:"members"`
}

// LoadConfig reads and validates the TOML file at path. Unknown keys are errors.
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	var raw fileConfig
	meta, err := toml.NewDecoder(f).Decode(&raw)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}

	var missing []string
	if raw.Listen == "" {
		missing = append(missing, "listen")
	}
	if raw.Database == "" {
		missing = append(missing, "database")
	}
	if len(raw.Members) == 0 {
		missing = append(missing, "members (at least one npub)")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("%s: missing required %v", path, missing)
	}

	members := make([]nostr.PubKey, 0, len(raw.Members))
	for _, entry := range raw.Members {
		pk, err := decodeNpub(entry)
		if err != nil {
			return Config{}, fmt.Errorf("%s: members: %w", path, err)
		}
		members = append(members, pk)
	}

	return Config{Name: raw.Name, Listen: raw.Listen, Database: raw.Database, Members: members}, nil
}

func decodeNpub(s string) (nostr.PubKey, error) {
	prefix, value, err := nip19.Decode(s)
	if err != nil {
		return nostr.ZeroPK, fmt.Errorf("%q is not an npub: %w", s, err)
	}
	pk, ok := value.(nostr.PubKey)
	if prefix != "npub" || !ok {
		return nostr.ZeroPK, fmt.Errorf("%q is an %s, an npub is required", s, prefix)
	}
	return pk, nil
}
