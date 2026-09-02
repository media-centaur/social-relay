package relay_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip42"
	"github.com/coder/websocket"
)

const wait = 5 * time.Second

// client is a raw NIP-01 WebSocket client. It mirrors what the app does on the wire:
// answer the AUTH challenge, wait for its OK, then REQ and EVENT. The module's own
// client performs AUTH asynchronously and races its challenge field, so it is not used.
type client struct {
	t         *testing.T
	url       string
	conn      *websocket.Conn
	challenge string
}

func dial(t *testing.T, url string) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return &client{t: t, url: url, conn: conn}
}

func (c *client) send(env json.Marshaler) {
	c.t.Helper()
	body, err := json.Marshal(env)
	if err != nil {
		c.t.Fatalf("marshal %T: %v", env, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, body); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

// read returns the next envelope, recording and skipping AUTH challenges so callers
// see only the messages they are waiting for.
func (c *client) read() nostr.Envelope {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	for {
		_, body, err := c.conn.Read(ctx)
		if err != nil {
			c.t.Fatalf("read: %v", err)
		}
		env, err := nostr.ParseMessage(string(body))
		if err != nil {
			c.t.Fatalf("parse %s: %v", body, err)
		}
		if auth, ok := env.(*nostr.AuthEnvelope); ok && auth.Challenge != nil {
			c.challenge = *auth.Challenge
			continue
		}
		return env
	}
}

// readChallenge waits for the relay's AUTH challenge without consuming other messages.
func (c *client) readChallenge() string {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	for c.challenge == "" {
		_, body, err := c.conn.Read(ctx)
		if err != nil {
			c.t.Fatalf("waiting for AUTH challenge: %v", err)
		}
		env, err := nostr.ParseMessage(string(body))
		if err != nil {
			c.t.Fatalf("parse %s: %v", body, err)
		}
		auth, ok := env.(*nostr.AuthEnvelope)
		if !ok || auth.Challenge == nil {
			c.t.Fatalf("expected AUTH challenge first, got %s", body)
		}
		c.challenge = *auth.Challenge
	}
	return c.challenge
}

func (c *client) readOK(id nostr.ID) (bool, string) {
	c.t.Helper()
	env := c.read()
	ok, is := env.(*nostr.OKEnvelope)
	if !is || ok.EventID != id {
		c.t.Fatalf("expected OK for %s, got %#v", id, env)
	}
	return ok.OK, ok.Reason
}

// auth answers the challenge as sk, naming the dialled URL in the relay tag.
func (c *client) auth(sk nostr.SecretKey) (bool, string) {
	c.t.Helper()
	return c.authAs(sk, c.url)
}

// authAs answers the challenge as sk with an explicit relay tag and returns the verdict.
func (c *client) authAs(sk nostr.SecretKey, relayURL string) (bool, string) {
	c.t.Helper()
	evt := nip42.CreateUnsignedAuthEvent(c.readChallenge(), sk.Public(), relayURL)
	if err := evt.Sign(sk); err != nil {
		c.t.Fatalf("sign auth: %v", err)
	}
	c.send(nostr.AuthEnvelope{Event: evt})
	return c.readOK(evt.ID)
}

func (c *client) publish(evt nostr.Event) (bool, string) {
	c.t.Helper()
	c.send(nostr.EventEnvelope{Event: evt})
	return c.readOK(evt.ID)
}

// request sends a REQ and collects stored events until EOSE. If the relay answers
// CLOSED instead, the reason is returned and events is nil.
func (c *client) request(id string, filter nostr.Filter) (events []nostr.Event, closed string) {
	c.t.Helper()
	c.send(nostr.ReqEnvelope{SubscriptionID: id, Filters: []nostr.Filter{filter}})
	for {
		switch env := c.read().(type) {
		case *nostr.EventEnvelope:
			events = append(events, env.Event)
		case *nostr.EOSEEnvelope:
			return events, ""
		case *nostr.ClosedEnvelope:
			return nil, env.Reason
		default:
			c.t.Fatalf("unexpected %#v while waiting for EOSE", env)
		}
	}
}

// readEvent waits for a live EVENT on subscription id.
func (c *client) readEvent(id string) nostr.Event {
	c.t.Helper()
	env := c.read()
	evt, ok := env.(*nostr.EventEnvelope)
	if !ok || evt.SubscriptionID == nil || *evt.SubscriptionID != id {
		c.t.Fatalf("expected EVENT on %s, got %#v", id, env)
	}
	return evt.Event
}

