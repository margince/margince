// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// This file is the Microsoft 365 calendar provider I/O: the read-only Graph
// calendar calls the connector needs. The OAuth2 handshake and the owner lookup
// are the SAME Microsoft plumbing the mail connector uses (capture/graph) —
// identical endpoints, identical consent parameters, a different scope — so
// they are reused rather than copied. This file owns only the calendar-specific
// endpoint (the calendarView delta walk) and its sentinel.

package graphcal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// graphAPIBase is Microsoft Graph's v1.0 root; overridable via NewAPI for
// tests. The connector reads the signed-in user's default calendar only.
const graphAPIBase = "https://graph.microsoft.com/v1.0"

// httpTimeout bounds every Microsoft call so a stalled request cannot pin the
// fleet-wide sync poller (http.DefaultClient has no timeout).
const httpTimeout = 30 * time.Second

// calendarReadScope is the single delegated permission the read-only calendar
// connector requests, beside the profile lookup and the refresh token. No
// write, no shared calendars.
const calendarReadScope = "Calendars.Read"

// The window the calendar view covers. Graph's calendarView needs a bounded
// range and the delta then tracks THAT range, so these are not merely a first
// pull's bound — they are what the standing connection watches.
//
// Backwards matches the Google connector's 90-day initial backfill: enough to
// give a new connection real history without streaming a decade of standups.
// Forwards is a year because a calendar's value is largely ahead of it, and a
// meeting booked further out than that will still be captured — the window
// slides on every re-anchor, so it arrives once it comes within range.
const (
	viewBackwards = 90 * 24 * time.Hour
	viewForwards  = 365 * 24 * time.Hour
)

// pageSize bounds one delta page; the connector pages until Graph returns the
// deltaLink that anchors the next incremental pull.
const pageSize = 100

// ErrAuthRejected and ErrUnreachable are the shared Microsoft transport
// sentinels, re-exported so this package's callers and tests keep a single
// graphcal-local name.
var (
	ErrAuthRejected = graph.ErrAuthRejected
	ErrUnreachable  = graph.ErrUnreachable
)

// ErrDeltaGone marks a deltaLink Graph no longer honors (HTTP 410 Gone with a
// resync hint); Sync falls back to a bounded re-anchor rather than failing —
// the calendar analogue of the mail connector's own delta expiry.
var ErrDeltaGone = fmt.Errorf("graphcal: delta cursor no longer valid: %w", connector.ErrCursorGone)

// OAuth is the OAuth2 handshake surface the connector consumes — the shared
// Microsoft shape. compose builds one concrete client and injects it here, so
// this package owns no duplicate token plumbing.
type OAuth = graph.OAuth

// OAuthConfig wires the calendar OAuth client. Tenant/AuthURL/TokenURL default
// to Microsoft's own endpoints; tests override TokenURL.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	Tenant       string
	AuthURL      string
	TokenURL     string
}

// NewOAuth builds the calendar OAuth client on the SAME Microsoft handshake the
// mail connector uses, with the calendar scopes.
//
// It is a SEPARATE Microsoft authorization from the mailbox's, requesting the
// calendar permission alone, so a person can connect one without the other and
// disconnect either — the same boundary the Google pair keeps. Reusing the
// handshake is deliberate (one copy of Microsoft's consent parameters and its
// scope-in-token-form requirement); reusing the NAME is not, so this passes its
// own, and a calendar failure reads as the calendar's in the roster.
//
//nolint:ireturn // returns the OAuth seam by design — the connector holds it as an interface so tests substitute a stub
func NewOAuth(cfg OAuthConfig) OAuth {
	return graph.NewOAuth(graph.OAuthConfig{
		Provider:     connectorName,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Tenant:       cfg.Tenant,
		Scopes:       Scopes(),
		AuthURL:      cfg.AuthURL,
		TokenURL:     cfg.TokenURL,
	})
}

// Scopes are the delegated permissions a calendar connection requests:
// offline_access for the refresh token, User.Read to resolve whose calendar it
// is, and calendar read. Exported so compose advertises exactly what NewOAuth
// asks for rather than a second list that can drift from it.
func Scopes() []string { return []string{"offline_access", "User.Read", calendarReadScope} }

// API is the read-only Graph calendar surface the connector uses. All calls
// take a short-lived access token (minted from the refresh token per Sync).
type API interface {
	// Owner returns the address of the signed-in account — the
	// internal-vs-external anchor. It is the SAME /me lookup the mail connector
	// makes, reused rather than reimplemented.
	Owner(ctx context.Context, accessToken string) (email string, err error)
	// ViewInitial starts a fresh calendarView delta over the window anchored at
	// now, walks it to completion, and returns the raw event resources plus the
	// deltaLink the next ViewDelta resumes from.
	ViewInitial(ctx context.Context, accessToken string) (events [][]byte, deltaLink string, err error)
	// ViewDelta resumes from a stored deltaLink and returns the events changed
	// since plus the advanced link; ErrDeltaGone if Graph no longer honors it.
	ViewDelta(ctx context.Context, accessToken, deltaLink string) (events [][]byte, newDeltaLink string, err error)
}

type httpAPI struct {
	client *http.Client
	base   string
	// mail is the shared Microsoft profile lookup — the same GET /me the mail
	// connector makes. Held rather than reimplemented so an installation whose
	// mailbox and calendar are the same account can never be told two different
	// things about whose account it is.
	mail graph.API
	// now anchors the calendar window; a field so tests pin the clock.
	now func() time.Time
}

// NewAPI builds the Graph calendar client over the given HTTP client and base
// URL (default Microsoft when base is empty; tests pass an httptest base).
//
//nolint:ireturn // returns the API seam by design — the connector holds it as an interface so tests substitute a stub
func NewAPI(client *http.Client, base string) API {
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	if base == "" {
		base = graphAPIBase
	}
	return &httpAPI{client: client, base: base, mail: graph.NewAPI(client, base), now: time.Now}
}

func (a *httpAPI) Owner(ctx context.Context, accessToken string) (string, error) {
	return a.mail.Profile(ctx, accessToken)
}

func (a *httpAPI) ViewInitial(ctx context.Context, accessToken string) ([][]byte, string, error) {
	now := a.now().UTC()
	q := url.Values{
		"startDateTime": {now.Add(-viewBackwards).Format(time.RFC3339)},
		"endDateTime":   {now.Add(viewForwards).Format(time.RFC3339)},
	}
	return a.walk(ctx, accessToken, a.base+"/me/calendarView/delta?"+q.Encode())
}

func (a *httpAPI) ViewDelta(ctx context.Context, accessToken, deltaLink string) ([][]byte, string, error) {
	if err := a.checkContinuation(deltaLink); err != nil {
		return nil, "", err
	}
	return a.walk(ctx, accessToken, deltaLink)
}

// deltaPage is one page of a calendarView delta. Items are kept as raw JSON so
// each event's original bytes reach the Sink as evidence unchanged.
type deltaPage struct {
	Value     []json.RawMessage `json:"value"`
	NextLink  string            `json:"@odata.nextLink"`  //nolint:tagliatelle // Microsoft's OData wire format; must match to decode
	DeltaLink string            `json:"@odata.deltaLink"` //nolint:tagliatelle // Microsoft's OData wire format; must match to decode
}

// walk follows a delta from the given URL to its end, collecting the raw event
// resources and the terminal deltaLink.
//
// A round that closes with NEITHER link is refused as unreachable rather than
// persisted: an empty cursor would force a full re-anchor every cycle, which is
// a silent, permanent doubling of the calendar's cost that nothing would
// report.
func (a *httpAPI) walk(ctx context.Context, accessToken, startURL string) ([][]byte, string, error) {
	var events [][]byte
	next := startURL
	for next != "" {
		var page deltaPage
		if err := a.get(ctx, accessToken, next, &page); err != nil {
			return nil, "", err
		}
		for _, item := range page.Value {
			events = append(events, []byte(item))
		}
		if page.NextLink == "" {
			if page.DeltaLink == "" {
				return nil, "", fmt.Errorf("graphcal: the delta round closed without a link: %w", ErrUnreachable)
			}
			return events, page.DeltaLink, nil
		}
		if err := a.checkContinuation(page.NextLink); err != nil {
			return nil, "", err
		}
		next = page.NextLink
	}
	return nil, "", fmt.Errorf("graphcal: the delta round closed without a link: %w", ErrUnreachable)
}

// checkContinuation refuses a server-supplied continuation URL that does not
// point at the configured Graph base.
//
// nextLink and deltaLink are URLs this client is told to fetch WITH THE
// ACCESS TOKEN ATTACHED, and one of them (the deltaLink) is persisted and
// followed on every later cycle. A link pointing elsewhere would send the
// mailbox's bearer token to whatever host produced it, so the origin is checked
// before the request rather than trusted because the previous hop was Graph.
func (a *httpAPI) checkContinuation(link string) error {
	// A PREFIX alone is not the base: "https://graph.microsoft.com/v1.0evil.test/…"
	// starts with it and is a different host entirely. What follows the base
	// must be a path or a query, or there is nothing left at all.
	if link != a.base &&
		!strings.HasPrefix(link, a.base+"/") &&
		!strings.HasPrefix(link, a.base+"?") {
		return fmt.Errorf("graphcal: continuation link does not point at the graph api: %w", ErrUnreachable)
	}
	return nil
}

// get performs an authorized GET and JSON-decodes into out, mapping a non-2xx
// onto the shared connector vocabulary. A 410 is the expired delta.
func (a *httpAPI) get(ctx context.Context, accessToken, fullURL string, out *deltaPage) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return fmt.Errorf("graphcal: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Add("Prefer", "odata.maxpagesize="+strconv.Itoa(pageSize))
	// Ask for UTC rather than accepting whatever the mailbox is set to. Graph
	// otherwise answers in the user's own zone and may name it with a WINDOWS
	// id ("W. Europe Standard Time"), which Go's tzdata cannot resolve — and an
	// unresolvable zone costs the event its real start. The decoder keeps its
	// zero-time fallback for a zone that still does not resolve; this is what
	// stops that path being the common case rather than the rare one.
	req.Header.Add("Prefer", `outlook.timezone="UTC"`)
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("graphcal: request: %w", ErrUnreachable)
	}
	//craft:ignore swallowed-errors best-effort close of the response body — the decoded result is what matters
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxPageLen))
	// Classify on status first: a 429/401 must be honored even if the body read
	// failed. Only on an otherwise-OK response does a read failure matter — a
	// truncated-but-valid-JSON prefix must never pass as a complete page.
	if err := classify(resp); err != nil {
		return err
	}
	if readErr != nil {
		return fmt.Errorf("graphcal: reading response: %w", ErrUnreachable)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("graphcal: decoding response: %w", ErrUnreachable)
	}
	return nil
}

// maxPageLen bounds one delta page's bytes, so a provider answering with an
// unbounded body cannot exhaust the worker's memory.
const maxPageLen = 8 << 20 // 8 MiB

// classify maps a non-2xx calendar response onto the shared connector
// vocabulary: 410 is the expired delta, 429 honors Retry-After, 401/403 parks
// the credential, anything else backs off. Microsoft's raw text is never
// surfaced to the caller.
func classify(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusGone:
		return ErrDeltaGone
	case resp.StatusCode == http.StatusTooManyRequests:
		return &connector.RateLimitedError{RetryAfter: retryafter.Of(resp)}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("graphcal: microsoft refused the calendar credential: %w", ErrAuthRejected)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("graphcal: microsoft answered %d: %w", resp.StatusCode, ErrUnreachable)
	}
	return nil
}
