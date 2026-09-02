// Command social-relay runs the allowlist relay described by a TOML config file.
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
		log.Printf("social-relay %s listening on %s, service URL %s, %d members", buildVersion(), cfg.Listen, cfg.ServiceURL, len(cfg.Members))
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
