// Command social-relay runs the allowlist relay described by a TOML config file, and
// manages a running relay's members as an admin:
//
//	social-relay [-config relay.toml]
//	social-relay -version
//	social-relay members list|add <npub> [reason]|remove <npub> [reason]  -relay wss://...
//
// The members subcommand signs with the admin key in SOCIAL_RELAY_ADMIN_KEY (nsec or hex).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"

	"github.com/media-centaur/social-relay/internal/manage"
	"github.com/media-centaur/social-relay/internal/relay"
)

// version is set by the release build (-ldflags "-X main.version=v1.2.3"). Local builds
// fall back to the module version Go stamps from the checkout, "(devel)" when untagged.
var version string

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "members" {
		if err := members(os.Args[2:]); err != nil {
			if errors.Is(err, errUsage) {
				fmt.Fprintln(os.Stderr, membersUsage)
			} else {
				fmt.Fprintln(os.Stderr, err)
			}
			os.Exit(1)
		}
		return
	}
	serve()
}

func serve() {
	configPath := flag.String("config", "relay.toml", "path of the TOML config file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildVersion())
		return
	}

	cfg, err := relay.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	r, err := relay.New(buildVersion(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	srv := &http.Server{Addr: cfg.Listen, Handler: r}
	go func() {
		log.Printf("social-relay %s listening on %s, service URL %s, %d admins", buildVersion(), cfg.Listen, cfg.ServiceURL, len(cfg.Admins))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

const membersUsage = `usage: social-relay members -relay <url> list
       social-relay members -relay <url> add <npub> [reason]
       social-relay members -relay <url> remove <npub> [reason]

Signs as the admin key in SOCIAL_RELAY_ADMIN_KEY (nsec1... or 64 hex characters).`

var errUsage = errors.New("invalid arguments")

func members(args []string) error {
	fs := flag.NewFlagSet("members", flag.ContinueOnError)
	relayURL := fs.String("relay", "", "the relay's service URL (ws:// or wss://)")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, membersUsage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *relayURL == "" || len(rest) == 0 {
		return errUsage
	}

	key, err := adminKey(os.Getenv("SOCIAL_RELAY_ADMIN_KEY"))
	if err != nil {
		return err
	}
	client, err := manage.New(*relayURL, key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch rest[0] {
	case "list":
		list, err := client.List(ctx)
		if err != nil {
			return err
		}
		for _, entry := range list {
			fmt.Printf("%s\t%s\n", nip19.EncodeNpub(entry.PubKey), entry.Reason)
		}
		return nil
	case "add", "remove":
		if len(rest) < 2 {
			return errUsage
		}
		pk, err := decodeNpub(rest[1])
		if err != nil {
			return err
		}
		reason := ""
		if len(rest) > 2 {
			reason = rest[2]
		}
		if rest[0] == "add" {
			if err := client.Allow(ctx, pk, reason); err != nil {
				return err
			}
			fmt.Printf("allowed %s\n", rest[1])
		} else {
			if err := client.Unallow(ctx, pk, reason); err != nil {
				return err
			}
			fmt.Printf("removed %s\n", rest[1])
		}
		return nil
	default:
		return errUsage
	}
}

func adminKey(s string) (nostr.SecretKey, error) {
	if s == "" {
		return nostr.SecretKey{}, errors.New("SOCIAL_RELAY_ADMIN_KEY is not set; it must hold an admin's nsec or hex secret key")
	}
	if prefix, value, err := nip19.Decode(s); err == nil {
		sk, ok := value.(nostr.SecretKey)
		if prefix != "nsec" || !ok {
			return nostr.SecretKey{}, fmt.Errorf("SOCIAL_RELAY_ADMIN_KEY is an %s, an nsec is required", prefix)
		}
		return sk, nil
	}
	sk, err := nostr.SecretKeyFromHex(s)
	if err != nil {
		return nostr.SecretKey{}, errors.New("SOCIAL_RELAY_ADMIN_KEY is neither an nsec nor a hex secret key")
	}
	return sk, nil
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
