// Package manage is the client side of the relay's NIP-86 management API: signed
// JSON-RPC over HTTP, authenticated with a NIP-98 event. The CLI and the tests use it.
package manage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip86"
)

// Client manages one relay as one admin.
type Client struct {
	relayURL string // normalised ws:// or wss:// form, what the relay compares the u tag against
	endpoint string // http:// or https:// form the request is posted to
	key      nostr.SecretKey
	http     *http.Client
}

// New prepares a client for relayURL (ws:// or wss://) signing as key.
func New(relayURL string, key nostr.SecretKey) (*Client, error) {
	normalized := nostr.NormalizeURL(relayURL)
	if normalized == "" {
		return nil, fmt.Errorf("invalid relay URL %q", relayURL)
	}
	return &Client{
		relayURL: normalized,
		endpoint: "http" + strings.TrimPrefix(normalized, "ws"),
		key:      key,
		http:     http.DefaultClient,
	}, nil
}

// Allow adds pubkey to the relay's members.
func (c *Client) Allow(ctx context.Context, pubkey nostr.PubKey, reason string) error {
	_, err := c.call(ctx, "allowpubkey", pubkey.Hex(), reason)
	return err
}

// Unallow removes pubkey from the relay's members.
func (c *Client) Unallow(ctx context.Context, pubkey nostr.PubKey, reason string) error {
	_, err := c.call(ctx, "unallowpubkey", pubkey.Hex(), reason)
	return err
}

// List returns every key allowed to read and write, admins included.
func (c *Client) List(ctx context.Context) ([]nip86.PubKeyReason, error) {
	result, err := c.call(ctx, "listallowedpubkeys")
	if err != nil {
		return nil, err
	}
	var list []nip86.PubKeyReason
	if err := json.Unmarshal(result, &list); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	return list, nil
}

func (c *Client) call(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	body, err := json.Marshal(nip86.Request{Method: method, Params: params})
	if err != nil {
		return nil, err
	}

	payloadHash := sha256.Sum256(body)
	auth := nostr.Event{
		Kind:      nostr.KindHTTPAuth,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"u", c.relayURL},
			{"method", http.MethodPost},
			{"payload", nostr.HexEncodeToString(payloadHash[:])},
		},
	}
	if err := auth.Sign(c.key); err != nil {
		return nil, fmt.Errorf("sign auth event: %w", err)
	}
	authJSON, err := json.Marshal(auth)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/nostr+json+rpc")
	req.Header.Set("Authorization", "Nostr "+base64.StdEncoding.EncodeToString(authJSON))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("%s answered %d: %.200s", c.endpoint, resp.StatusCode, raw)
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	return response.Result, nil
}
