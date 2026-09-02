package relay_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fiatjaf.com/nostr"

	"github.com/media-centaur/social-relay/internal/manage"
	"github.com/media-centaur/social-relay/internal/relay"
)

func manager(t *testing.T, url string, sk nostr.SecretKey) *manage.Client {
	t.Helper()
	c, err := manage.New(url, sk)
	if err != nil {
		t.Fatalf("manage.New: %v", err)
	}
	return c
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), wait)
	t.Cleanup(cancel)
	return c
}

func TestAdminAllowsAKeyWhichCanThenReadAndWrite(t *testing.T) {
	admin, friend := nostr.Generate(), nostr.Generate()
	url := startRelay(t, admin.Public())

	c := connectAs(t, url, friend)
	if _, closed := c.request("feed", feedFilter(admin.Public())); !strings.HasPrefix(closed, "restricted: ") {
		t.Fatalf("before allow: CLOSED = %q, want restricted:", closed)
	}

	if err := manager(t, url, admin).Allow(ctx(t), friend.Public(), "test friend"); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// The same socket, no reconnect: membership is checked per request.
	if _, closed := c.request("feed", feedFilter(admin.Public())); closed != "" {
		t.Fatalf("after allow: CLOSED = %q, want stored events and EOSE", closed)
	}
	if ok, reason := c.publish(recommendation(t, friend, "tmdb:movie:1", nostr.Now())); !ok {
		t.Fatalf("after allow: publish refused: %s", reason)
	}
}

func TestUnallowedKeyIsRestrictedAndReceivesNoLiveEvents(t *testing.T) {
	admin, friend := nostr.Generate(), nostr.Generate()
	url := startRelay(t, admin.Public())
	m := manager(t, url, admin)
	if err := m.Allow(ctx(t), friend.Public(), ""); err != nil {
		t.Fatalf("allow: %v", err)
	}

	c := connectAs(t, url, friend)
	if _, closed := c.request("feed", feedFilter(admin.Public())); closed != "" {
		t.Fatalf("subscription closed: %s", closed)
	}

	if err := m.Unallow(ctx(t), friend.Public(), "left the group"); err != nil {
		t.Fatalf("unallow: %v", err)
	}

	// khatru delivers live events synchronously before answering the publisher's OK,
	// so once the admin has its OK the removed socket has either received the event
	// or never will. Its next frame must therefore be the CLOSED for this REQ.
	mustPublish(t, connectAs(t, url, admin), recommendation(t, admin, "tmdb:movie:1", nostr.Now()))
	c.send(nostr.ReqEnvelope{SubscriptionID: "again", Filters: []nostr.Filter{feedFilter(admin.Public())}})
	switch env := c.read().(type) {
	case *nostr.ClosedEnvelope:
		if !strings.HasPrefix(env.Reason, "restricted: ") {
			t.Errorf("after unallow: CLOSED = %q, want restricted:", env.Reason)
		}
	case *nostr.EventEnvelope:
		t.Fatalf("live event %s delivered after unallow", env.Event.ID)
	default:
		t.Fatalf("unexpected %#v after unallow", env)
	}
	if ok, reason := c.publish(recommendation(t, friend, "tmdb:movie:2", nostr.Now())); ok || !strings.HasPrefix(reason, "restricted: ") {
		t.Errorf("after unallow: OK = %v %q, want false with restricted:", ok, reason)
	}
}

func TestOnlyAdminsMayManage(t *testing.T) {
	admin, friend, outsider := nostr.Generate(), nostr.Generate(), nostr.Generate()
	url := startRelay(t, admin.Public())
	if err := manager(t, url, admin).Allow(ctx(t), friend.Public(), ""); err != nil {
		t.Fatalf("allow: %v", err)
	}

	for name, sk := range map[string]nostr.SecretKey{"member": friend, "outsider": outsider} {
		err := manager(t, url, sk).Allow(ctx(t), nostr.Generate().Public(), "")
		if err == nil || !strings.Contains(err.Error(), "restricted: ") {
			t.Errorf("%s managing: err = %v, want restricted:", name, err)
		}
	}
}

func TestAllowedKeysSurviveRestart(t *testing.T) {
	admin, friend := nostr.Generate(), nostr.Generate()
	dbPath := filepath.Join(t.TempDir(), "events.db")
	cfg := relay.Config{Name: "test relay", Database: dbPath, Admins: []nostr.PubKey{admin.Public()}}

	first, err := relay.New(testVersion, cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(first)
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	if err := manager(t, url, admin).Allow(ctx(t), friend.Public(), ""); err != nil {
		t.Fatalf("allow: %v", err)
	}
	srv.Close()
	first.Close()

	second, err := relay.New(testVersion, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.Close)
	srv = httptest.NewServer(second)
	t.Cleanup(srv.Close)
	url = "ws" + strings.TrimPrefix(srv.URL, "http")

	if _, closed := connectAs(t, url, friend).request("feed", feedFilter(admin.Public())); closed != "" {
		t.Fatalf("after restart: CLOSED = %q, want member access", closed)
	}
}

func TestListIncludesAdminsAndAllowedKeys(t *testing.T) {
	admin, friend := nostr.Generate(), nostr.Generate()
	url := startRelay(t, admin.Public())
	m := manager(t, url, admin)
	if err := m.Allow(ctx(t), friend.Public(), "test friend"); err != nil {
		t.Fatalf("allow: %v", err)
	}

	list, err := m.List(ctx(t))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[nostr.PubKey]string{}
	for _, entry := range list {
		got[entry.PubKey] = entry.Reason
	}
	if got[admin.Public()] != "admin" {
		t.Errorf("admin listed with reason %q, want %q", got[admin.Public()], "admin")
	}
	if got[friend.Public()] != "test friend" {
		t.Errorf("friend listed with reason %q, want %q", got[friend.Public()], "test friend")
	}
	if len(got) != 2 {
		t.Errorf("list has %d entries, want 2", len(got))
	}
}

func TestUnallowingAnAdminFails(t *testing.T) {
	admin := nostr.Generate()
	url := startRelay(t, admin.Public())

	err := manager(t, url, admin).Unallow(ctx(t), admin.Public(), "")
	if err == nil || !strings.Contains(err.Error(), "admin") {
		t.Fatalf("err = %v, want an error explaining admins are set in the config", err)
	}
}
