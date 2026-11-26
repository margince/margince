// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/margince/margince/backend/internal/platform/safedownload"
	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
)

// GraphBase and GraphVersion are Meta's production origin and the API version
// this layer was written against. NewClient takes both so tests drive a local
// server and an upgrade is one value.
const (
	GraphBase    = "https://graph.facebook.com"
	GraphVersion = "v27.0"
)

const (
	httpTimeout      = 30 * time.Second
	maxResponseBytes = 1 << 20
)

// Client calls the WhatsApp Cloud API. The access token is passed per call, as
// one client serves every workspace's connection.
type Client struct {
	http       *http.Client
	base       string
	version    string
	mediaHosts []string
	policy     safedownload.Policy
	control    func(network, address string, c syscall.RawConn) error
}

// DefaultMediaHosts are the hosts media may be downloaded from. A pattern
// "*.example.com" matches any subdomain of example.com.
var DefaultMediaHosts = []string{"lookaside.fbsbx.com", "*.fbsbx.com", "*.whatsapp.net"}

// Option adjusts a Client.
type Option func(*Client)

// WithMediaHosts replaces the media download allowlist; tests use it to admit
// a local server.
func WithMediaHosts(patterns ...string) Option {
	return func(c *Client) { c.mediaHosts = patterns }
}

// WithoutDialGuardForTest lets the client dial a server on the loopback.
func WithoutDialGuardForTest() Option {
	return func(c *Client) { c.control = safedownload.NoDialGuardForTest }
}

// NewClient builds a Client. A nil client gets one with a bounded timeout;
// empty base or version fall back to GraphBase and GraphVersion.
func NewClient(hc *http.Client, base, version string, opts ...Option) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: httpTimeout}
	}
	if base == "" {
		base = GraphBase
	}
	if version == "" {
		version = GraphVersion
	}
	c := &Client{http: hc, base: strings.TrimRight(base, "/"), version: version, mediaHosts: DefaultMediaHosts}
	for _, opt := range opts {
		opt(c)
	}
	// The caller's client is copied, so its redirect policy stays its own. Every
	// hop is checked like the first request, because a redirect from an allowed
	// host could otherwise carry the bearer token to any other.
	graph, err := url.Parse(c.base)
	if err != nil {
		graph = &url.URL{}
	}
	c.policy = safedownload.Policy{Hosts: c.mediaHosts, RedirectHosts: []string{graph.Host}, Control: c.control}
	c.http = c.policy.Client(hc)
	return c
}

// allowedMediaURL reports whether raw is an https URL on an allowed host.
func (c *Client) allowedMediaURL(raw string) bool { return c.policy.Allows(raw) }

// lookasideHost is the one media host Meta's documentation has the access token
// sent to. Every other media host serves signed URLs and gets no credential.
const lookasideHost = "lookaside.fbsbx.com"

// bearerHost reports whether the access token may go to raw: the Graph API host
// or Meta's lookaside host, and no other.
func (c *Client) bearerHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	graph, err := url.Parse(c.base)
	if err == nil && strings.EqualFold(u.Host, graph.Host) {
		return true
	}
	return strings.EqualFold(u.Hostname(), lookasideHost)
}

// Download is a fetched attachment.
type Download struct {
	Data     []byte
	MimeType string
}

type mediaInfo struct {
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	FileSize int64  `json:"file_size"`
}

// DownloadMedia fetches the attachment mediaID names: first its metadata, then
// the bytes from the URL the metadata gives, both with the bearer token.
// maxBytes bounds the body. wantSHA256, when non-empty (the checksum the
// webhook carried), and the checksum in Meta's metadata are both verified.
func (c *Client) DownloadMedia(ctx context.Context, token, mediaID string, maxBytes int64, wantSHA256 string) (Download, error) {
	raw, err := c.do(ctx, http.MethodGet, c.base+"/"+c.version+"/"+url.PathEscape(mediaID), token, nil, maxResponseBytes)
	if err != nil {
		return Download{}, err
	}
	var info mediaInfo
	if err := json.Unmarshal(raw, &info); err != nil || info.URL == "" {
		return Download{}, fmt.Errorf("whatsapp: media metadata unreadable: %w", ErrUnreachable)
	}
	if info.FileSize > maxBytes {
		return Download{}, ErrMediaTooLarge
	}
	dl, err := c.DownloadURL(ctx, token, info.URL, maxBytes, info.SHA256, wantSHA256)
	if err != nil {
		return Download{}, err
	}
	dl.MimeType = info.MimeType
	return dl, nil
}

// DownloadURL fetches media from a URL the payload or media metadata gave. The
// URL must be https on an allowed host, so a payload cannot aim the bearer
// token at another server. Every non-empty checksum in sums must match.
func (c *Client) DownloadURL(ctx context.Context, token, mediaURL string, maxBytes int64, sums ...string) (Download, error) {
	if !c.allowedMediaURL(mediaURL) {
		return Download{}, fmt.Errorf("whatsapp: media url not allowed: %w", ErrRejected)
	}
	if !c.bearerHost(mediaURL) {
		token = ""
	}
	data, err := c.do(ctx, http.MethodGet, mediaURL, token, nil, maxBytes)
	if err != nil {
		return Download{}, err
	}
	for _, want := range sums {
		if want != "" && !checksumMatches(data, want) {
			return Download{}, ErrMediaChecksum
		}
	}
	return Download{Data: data}, nil
}

// checksumMatches accepts the digest as hex (Graph's media metadata) or base64
// (as webhook payloads state it), because the two surfaces differ.
func checksumMatches(data []byte, want string) bool { return safedownload.SHA256Matches(data, want) }

// SendText sends one free-form text message from phoneNumberID to the
// customer's wa_id and returns Meta's message id. Outside the 24-hour window
// this fails with ErrOutsideWindow.
func (c *Client) SendText(ctx context.Context, token, phoneNumberID, to, text string) (string, error) {
	body, err := json.Marshal(struct {
		MessagingProduct string `json:"messaging_product"`
		RecipientType    string `json:"recipient_type"`
		To               string `json:"to"`
		Type             string `json:"type"`
		Text             struct {
			Body string `json:"body"`
		} `json:"text"`
	}{
		MessagingProduct: "whatsapp", RecipientType: "individual", To: to, Type: typeText,
		Text: struct {
			Body string `json:"body"`
		}{Body: text},
	})
	if err != nil {
		return "", fmt.Errorf("whatsapp: encoding the send request: %w", err)
	}
	raw, err := c.do(ctx, http.MethodPost, c.base+"/"+c.version+"/"+url.PathEscape(phoneNumberID)+"/messages", token, body, maxResponseBytes)
	if err != nil {
		return "", err
	}
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Messages) == 0 || out.Messages[0].ID == "" {
		// Meta accepted the call but named no message; the send state is unknown.
		return "", fmt.Errorf("whatsapp: send answer carried no message id: %w", ErrUnreachable)
	}
	return out.Messages[0].ID, nil
}

// do performs one authorized request and returns the body of a 2xx answer, or
// the classified error. It never wraps a transport error's cause: a *url.Error
// message carries the request URL.
func (c *Client) do(ctx context.Context, method, target, token string, body []byte, limit int64) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: the request could not be built: %w", ErrRejected)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := safedownload.Do(c.http, req, limit)
	switch {
	case errors.Is(err, safedownload.ErrRefused):
		return nil, fmt.Errorf("whatsapp: a redirect left the allowed hosts: %w", ErrRejected)
	case errors.Is(err, safedownload.ErrTooLarge):
		if limit == maxResponseBytes {
			return nil, fmt.Errorf("whatsapp: answer too large: %w", ErrUnreachable)
		}
		return nil, ErrMediaTooLarge
	case err != nil:
		return nil, fmt.Errorf("whatsapp: transport failure: %w", ErrUnreachable)
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, classify(resp.Status, graphErrorOf(resp.Body), retryafter.FromHeader(resp.Header))
	}
	return resp.Body, nil
}

// graphErrorOf reads Meta's error envelope; an unreadable body yields zeros,
// which classify then handles by status alone.
func graphErrorOf(data []byte) GraphError {
	var env struct {
		Error struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
			Subcode int    `json:"error_subcode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return GraphError{}
	}
	return GraphError{Code: env.Error.Code, Subcode: env.Error.Subcode, Message: env.Error.Message}
}

// IsRetryable reports whether the same call may succeed later without anyone
// changing configuration: a throttle or an unreachable Graph API.
func IsRetryable(err error) bool {
	return errors.Is(err, ErrUnreachable) || isRateLimited(err)
}
