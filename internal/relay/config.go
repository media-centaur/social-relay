package relay

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config is the operator's TOML file.
type Config struct {
	// Name is advertised in the NIP-11 relay information document.
	Name string `toml:"name"`
	// Listen is the plain-HTTP address the relay binds; TLS is the reverse proxy's job.
	Listen string `toml:"listen"`
	// Database is the path of the bbolt file holding every stored event.
	Database string `toml:"database"`
}

// LoadConfig reads and validates the TOML file at path. Unknown keys are errors.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	meta, err := toml.NewDecoder(f).Decode(&cfg)
	if err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}
	var missing []string
	if cfg.Listen == "" {
		missing = append(missing, "listen")
	}
	if cfg.Database == "" {
		missing = append(missing, "database")
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("%s: missing required %v", path, missing)
	}
	return cfg, errors.Join()
}
