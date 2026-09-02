package relay

import (
	"fmt"
	"net/url"
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
	// ServiceURL is the public ws:// or wss:// address members paste into the app. NIP-42
	// AUTH answers must name it in their relay tag, and the relay serves only its path.
	ServiceURL string
	// Admins may manage membership through the NIP-86 API and are members themselves.
	// Every other member is added at runtime and stored in the database.
	Admins []nostr.PubKey
}

// fileConfig mirrors the TOML file before validation. Members are npub strings there.
type fileConfig struct {
	Name       string   `toml:"name"`
	Listen     string   `toml:"listen"`
	Database   string   `toml:"database"`
	ServiceURL string   `toml:"service_url"`
	Admins     []string `toml:"admins"`
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
	if raw.ServiceURL == "" {
		missing = append(missing, "service_url")
	}
	if len(raw.Admins) == 0 {
		missing = append(missing, "admins (at least one npub)")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("%s: missing required %v", path, missing)
	}

	if err := checkServiceURL(raw.ServiceURL); err != nil {
		return Config{}, fmt.Errorf("%s: service_url: %w", path, err)
	}

	admins := make([]nostr.PubKey, 0, len(raw.Admins))
	for _, entry := range raw.Admins {
		pk, err := decodeNpub(entry)
		if err != nil {
			return Config{}, fmt.Errorf("%s: admins: %w", path, err)
		}
		admins = append(admins, pk)
	}

	return Config{
		Name:       raw.Name,
		Listen:     raw.Listen,
		Database:   raw.Database,
		ServiceURL: raw.ServiceURL,
		Admins:     admins,
	}, nil
}

func checkServiceURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
		return fmt.Errorf("%q must be a ws:// or wss:// URL with a host", s)
	}
	return nil
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
