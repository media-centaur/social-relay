// Command social-relay runs the allowlist relay described by a TOML config file.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/media-centaur/social-relay/internal/relay"
)

func main() {
	configPath := flag.String("config", "relay.toml", "path of the TOML config file")
	flag.Parse()

	cfg, err := relay.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	r, err := relay.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	srv := &http.Server{Addr: cfg.Listen, Handler: r}
	go func() {
		log.Printf("listening on %s", cfg.Listen)
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
