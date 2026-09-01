// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// This file is the Microsoft Graph provider I/O: the Microsoft identity
// platform OAuth2 handshake and the handful of read-only Graph mail calls the
// connector needs, hand-rolled over net/http so capture takes on no new module
// dependency. Both surfaces are interfaces (OAuth, API) so the connector's
// Sync/Authenticate are unit tested against a stub, and non-2xx responses map
// to sentinels the transport turns into clean 422/502 without echoing
// Microsoft's raw text.

package graph

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// httpTimeout bounds every Microsoft call so a stalled OAuth/Graph request
// can't pin an API callback or the fleet-wide sync poller (http.DefaultClient
// has no timeout).
const httpTimeout = 30 * time.Second

func boundedHTTPClient() *http.Client { return &http.Client{Timeout: httpTimeout} }

// Default Microsoft endpoints; overridable via OAuthConfig / NewAPI for tests.
// The identity endpoints are tenant-scoped ("common" serves any work/school or
// personal account); the Graph API base is tenant-less — the access token
// carries the identity.
const (
	msAuthURLFormat  = "https://login.microsoftonline.com/%s/oauth2/v2.0/authorize"
	msTokenURLFormat = "https://login.microsoftonline.com/%s/oauth2/v2.0/token" //nolint:gosec // G101 false positive: Microsoft's public OAuth token *endpoint URL*, not a credential
	graphAPIBase     = "https://graph.microsoft.com/v1.0"

	// defaultTenant is the multi-tenant identity endpoint: any Microsoft 365
	// organization (and personal accounts) can consent. A single-tenant
	// deployment narrows it to its own tenant id via OAuthConfig.Tenant.
	defaultTenant = "common"

	paramClientID = "client_id"
	paramScope    = "scope"
	paramFilter   = "$filter"
	paramSelect   = "$select"
	paramTop      = "$top"
	paramCount    = "$count"

	// maxMIMELen bounds one message's fetched RFC822 bytes. A message over
	// this is refused rather than truncated (a truncated MIME blob is not
	// parseable, honest evidence) — the same cap discipline as IMAP.
	maxMIMELen = 8 << 20 // 8 MiB
)

// The package sentinels wrap the shared connector vocabulary (ADR-0063) so
// the registry classifies failures without knowing this provider: auth parks
// the connection, a rate limit honors Retry-After, unreachable backs off.

// ErrAuthRejected marks an OAuth failure Microsoft reported (bad/expired code,
// revoked refresh). The transport maps it to a 422 without echoing the raw
// provider error.
var ErrAuthRejected = fmt.Errorf("graph: the authorization was rejected: %w", connector.ErrAuthRejected)

// ErrUnreachable marks a transport-level failure reaching Microsoft (DNS, TCP,
// TLS, timeout, 5xx). The transport maps it to a 502.
var ErrUnreachable = fmt.Errorf("graph: could not reach Microsoft: %w", connector.ErrUnreachable)

// ErrDeltaGone marks a deltaLink Graph no longer honors (HTTP 410 Gone with a
// resync hint); Sync falls back to a bounded re-anchor rather than failing.
var ErrDeltaGone = fmt.Errorf("graph: delta cursor no longer valid: %w", connector.ErrCursorGone)

// Authorizer is the TOKEN half of the handshake: exchange an authorization code
// for a refresh token, and mint a fresh access token from a stored one.
//
// The narrower half a connector holds, split from OAuth for the same reason
// googleconn splits its own: an installation whose Entra app arrives at runtime
// resolves the app per call, and a per-call resolution cannot build a consent
// URL — AuthCodeURL takes no context and cannot report that the app is
// unreachable. So the transport keeps that half, resolving the app itself, and
// the connector holds this one.
type Authorizer interface {
	Exchange(ctx context.Context, code, redirectURI string) (oauthflow.TokenGrant, error)
	AccessToken(ctx context.Context, refreshToken string) (accessToken string, err error)
	// Refresh redeems the stored refresh token asking for exactly the scopes
	// THIS connection holds, and reports the rotation Microsoft performs on
	// every redemption.
	//
	// Both halves matter and neither is optional. Asking with the deployment's
	// configured list instead would stop an older mailbox CAPTURING the day a
	// scope is added — Microsoft refuses a refresh that names a permission the
	// grant never carried — and dropping the rotation ages the connection out
	// on Microsoft's own 90-day idle clock.
	Refresh(ctx context.Context, refreshToken string, granted []string) (oauthflow.TokenRefresh, error)
}

// OAuth is the full handshake surface: an Authorizer that can also send a person
// to Microsoft's consent screen. The transport holds this; a connector holds the
// narrower half.
type OAuth interface {
	Authorizer
	AuthCodeURL(state, redirectURI string) string
}

// API is the Graph mail surface the connector uses. Reading is what almost all
// of it does; the writes are named and few — the two send calls at the end,
// which ride the separate Mail.Send permission, and EnsureSubscription, which
// registers the change-notification subscription push arrives through. All
// calls take a short-lived access token (minted from the refresh token per
// Sync).
type API interface {
	// Profile returns the mailbox owner's address (mail, falling back to
	// userPrincipalName for a mailbox with no mail attribute).
	Profile(ctx context.Context, accessToken string) (email string, err error)
	// DeltaInit starts a fresh delta over ONE well-known folder, bounded to
	// messages received on or after the given instant, walks it to completion,
	// and returns the deltaLink the next Delta resumes from. A walk that ends
	// holding fewer messages than the folder reports for that window is
	// refused rather than returned: the watermark only moves forward, so a
	// short anchor would drop what it missed for good.
	//
	// Per folder rather than over the whole mailbox: /me/messages/delta would
	// also yield Drafts, Deleted Items and Junk. A draft was never sent, so
	// putting one on a customer's timeline would be recording a message that
	// does not exist.
	DeltaInit(ctx context.Context, accessToken, folder string, after time.Time) (ids []string, deltaLink string, err error)
	// Delta resumes a folder's delta from a stored deltaLink and returns the
	// message ids added/changed since plus the advanced deltaLink;
	// ErrDeltaGone if Graph no longer honors the link.
	Delta(ctx context.Context, accessToken, deltaLink string) (ids []string, newDeltaLink string, err error)
	// EnsureSubscription registers or renews the Graph change-notification
	// subscription pointing at notificationURL, carrying clientState, and
	// reports the subscription that now covers this mailbox.
	//
	// One call rather than a list/create/renew trio on this seam: which of
	// those a round performs is Microsoft's business, and a seam that spelled
	// the three would make every test double re-decide the renew-then-create
	// order the real one exists to hold.
	EnsureSubscription(ctx context.Context, accessToken, notificationURL, clientState string, deadline time.Time) (Subscription, error)
	// RenewSubscription extends the subscription named by id and answers its
	// new deadline. ErrSubscriptionGone when Microsoft no longer knows it,
	// which is the caller's cue to fall back to EnsureSubscription.
	//
	// A SECOND seam beside EnsureSubscription rather than a flag on it, because
	// the two answer different questions: Ensure asks "is there a subscription
	// for this mailbox", which without a handle costs a paged listing, and this
	// asks "extend THIS one", which is one call. A renewal that knows the id
	// should not have to pay for the question it already has the answer to.
	RenewSubscription(ctx context.Context, accessToken, id string, deadline time.Time) (Subscription, error)
	// GetMIME fetches one message as its RFC822 bytes (the /$value stream).
	GetMIME(ctx context.Context, accessToken, msgID string) (rfc822 []byte, err error)
	// EstimateAfter returns the provider-side count of messages received on
	// or after the given instant ($count=true) — the backfill preview's number.
	EstimateAfter(ctx context.Context, accessToken string, after time.Time) (int, error)
	// ListAfter returns one page of messages received on or after the given
	// instant; pageToken is the @odata.nextLink of the prior page ("" starts
	// the walk).
	ListAfter(ctx context.Context, accessToken string, after time.Time, pageToken string, pageSize int) (msgs []MessageRef, next string, err error)
	// SentFolderID returns the id of the mailbox's Sent Items well-known
	// folder, against which a message's ParentFolderID identifies mail the
	// authenticated owner sent.
	SentFolderID(ctx context.Context, accessToken string) (string, error)

	// SendMIME transmits one complete RFC822 message as the signed-in user.
	// Microsoft acknowledges the submission without naming a message id, so
	// this reports only whether it was accepted.
	SendMIME(ctx context.Context, accessToken string, rfc822 []byte) error
	// FindSentByMessageID resolves the sent copy of a message by the RFC822
	// identity it was asked to carry — the at-least-once retry guard, and the
	// only way to name a message this connector has just submitted.
	FindSentByMessageID(ctx context.Context, accessToken, unbracketedMessageID string) (msgID string, found bool, err error)
}

// MessageRef is one listed message: its id, plus the folder Graph filed it in.
// The folder is the T1 correspondence evidence (ADR-0072 §1) — Microsoft moves
// a message into Sent Items because the authenticated account sent it, which no
// amount of header forgery can imitate.
//
// ParentFolderID is never empty: ListAfter refuses a listed message that names
// no folder, so a caller comparing it against SentFolderID can never be
// comparing two absent values.
type MessageRef struct {
	ID             string
	ParentFolderID string
}

// OAuthConfig wires the OAuth client. Tenant defaults to "common";
// AuthURL/TokenURL default to Microsoft's tenant-scoped endpoints when empty
// (tests override TokenURL).
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	Tenant       string
	Scopes       []string
	AuthURL      string
	TokenURL     string
	// Provider names the connector in a handshake failure's error detail.
	// Empty means this package's own name. The Microsoft 365 CALENDAR connector
	// runs the identical handshake against the identical endpoints with
	// different scopes, so it reuses this constructor rather than keeping a
	// second copy of Microsoft's consent parameters; what it must not reuse is
	// the NAME, or its own failures would read as the mail connector's in the
	// logs and in the connection roster.
	Provider string
}

// NewOAuth builds the OAuth client, applying Microsoft's default endpoints
// (for the configured tenant) when the config leaves them empty. The
// handshake itself is the shared oauthflow; only Microsoft's tenant-scoped
// endpoints, consent parameters, and this package's sentinels differ.
//
//nolint:ireturn // returns the OAuth seam by design — the connector holds it as an interface so tests substitute a stub
func NewOAuth(cfg OAuthConfig) OAuth {
	tenant := cfg.Tenant
	if tenant == "" {
		tenant = defaultTenant
	}
	if cfg.AuthURL == "" {
		cfg.AuthURL = fmt.Sprintf(msAuthURLFormat, url.PathEscape(tenant))
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = fmt.Sprintf(msTokenURLFormat, url.PathEscape(tenant))
	}
	provider := cfg.Provider
	if provider == "" {
		provider = connectorName
	}
	return oauthflow.New(oauthflow.Config{
		Provider:     provider,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       cfg.Scopes,
		AuthURL:      cfg.AuthURL,
		TokenURL:     cfg.TokenURL,
		// Microsoft returns the code on the query (not fragment); the v2
		// endpoint requires the scope in every token form for a refresh token.
		// Microsoft rotates the refresh token on each redemption, and the
		// replacement is persisted: Refresh reports it and the connector hands
		// it to the registry's credential sink, which re-seals it. Without that
		// a mailbox idle past the ~90-day confidential-client lifetime had to
		// reconnect for no reason its owner could see.
		AuthParams:        map[string]string{"response_mode": "query"},
		ScopeInTokenForms: true,
		AuthRejected:      ErrAuthRejected,
		Unreachable:       ErrUnreachable,
	})
}

type httpAPI struct {
	client *http.Client
	base   string
}

// NewAPI builds the Graph REST client over the given HTTP client and base
// URL (default Microsoft when base is empty; tests pass an httptest base).
//
//nolint:ireturn // returns the API seam by design — the connector holds it as an interface so tests substitute a stub
func NewAPI(client *http.Client, base string) API {
	if client == nil {
		client = boundedHTTPClient()
	}
	if base == "" {
		base = graphAPIBase
	}
	return &httpAPI{client: client, base: base}
}

func (a *httpAPI) Profile(ctx context.Context, accessToken string) (string, error) {
	var out struct {
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"` //nolint:tagliatelle // Microsoft's wire format (camelCase); must match to decode
	}
	if _, err := a.get(ctx, accessToken, a.base+"/me", nil, &out); err != nil {
		return "", err
	}
	if out.Mail != "" {
		return out.Mail, nil
	}
	return out.UserPrincipalName, nil
}

func (a *httpAPI) sameAPIOrigin(link string) error {
	if !strings.HasPrefix(link, a.base+"/") && link != a.base {
		return fmt.Errorf("graph: continuation link does not point at the graph api")
	}
	return nil
}

func (a *httpAPI) GetMIME(ctx context.Context, accessToken, msgID string) ([]byte, error) {
	u := a.base + "/me/messages/" + url.PathEscape(msgID) + "/$value"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: building message request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graph: message %s: %w", msgID, ErrUnreachable)
	}
	//craft:ignore swallowed-errors best-effort close of the message body — the MIME bytes are already read below
	defer func() { _ = resp.Body.Close() }()
	// Read one byte past the cap so an oversized message is detected, not
	// silently truncated: LimitReader alone returns EOF exactly at the cap,
	// which would parse and persist a body missing its tail (the same
	// discipline the IMAP connector's readCapped uses).
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMIMELen+1))
	if err != nil {
		return nil, fmt.Errorf("graph: reading message %s: %w", msgID, ErrUnreachable)
	}
	if resp.StatusCode == http.StatusNotFound {
		// The message was deleted between delta enumeration and this fetch —
		// skip it (the cursor still advances) rather than retrying a message
		// that is permanently gone, which would wedge the whole sync.
		return nil, fmt.Errorf("graph: message %s no longer exists: %w", msgID, connector.ErrSkip)
	}
	if err := classifyStatus(resp, messageOp, body); err != nil {
		return nil, err
	}
	if len(body) > maxMIMELen {
		// Oversized: a truncated MIME blob is neither parseable nor honest
		// evidence, so it is refused (counted as a skip upstream), never
		// stored half-read.
		return nil, fmt.Errorf("graph: message %s exceeds the size cap: %w", msgID, connector.ErrSkip)
	}
	return body, nil
}

func (a *httpAPI) EstimateAfter(ctx context.Context, accessToken string, after time.Time) (int, error) {
	var out struct {
		Count int `json:"@odata.count"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	}
	q := url.Values{
		paramFilter: {receivedAfterFilter(after)},
		paramCount:  {"true"},
		paramSelect: {"id"},
		paramTop:    {"1"},
	}
	// $count=true needs the eventual-consistency header on Graph.
	hdr := http.Header{"ConsistencyLevel": {"eventual"}}
	if _, err := a.get(ctx, accessToken, a.base+"/me/messages?"+q.Encode(), hdr, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

func (a *httpAPI) ListAfter(ctx context.Context, accessToken string, after time.Time, pageToken string, pageSize int) ([]MessageRef, string, error) {
	u := pageToken
	if u == "" {
		q := url.Values{
			paramFilter: {receivedAfterFilter(after)},
			paramSelect: {"id,parentFolderId"},
			paramTop:    {strconv.Itoa(pageSize)},
		}
		u = a.base + "/me/messages?" + q.Encode()
	} else if err := a.sameAPIOrigin(u); err != nil {
		return nil, "", err
	}
	var out struct {
		Value []struct {
			ID             string `json:"id"`
			ParentFolderID string `json:"parentFolderId"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
		} `json:"value"`
		NextLink string `json:"@odata.nextLink"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	}
	if _, err := a.get(ctx, accessToken, u, nil, &out); err != nil {
		return nil, "", err
	}
	msgs := make([]MessageRef, 0, len(out.Value))
	for _, m := range out.Value {
		if m.ParentFolderID == "" {
			// Every Graph message lives in a folder, so a listed one naming none
			// is a truncated answer, not a message outside the hierarchy. It
			// cannot be captured on a guess in EITHER direction: attested, an
			// empty id would match an unresolved folder; un-attested, the
			// activity natural key would permanently discard evidence the owner
			// really did produce. Refusing the page keeps both off the table.
			return nil, "", fmt.Errorf("graph: message %s listed without a parent folder: %w", m.ID, ErrUnreachable)
		}
		msgs = append(msgs, MessageRef{ID: m.ID, ParentFolderID: m.ParentFolderID})
	}
	return msgs, out.NextLink, nil
}

// SentFolderID resolves the mailbox's Sent Items folder by its well-known
// name, which Graph accepts in place of the opaque id the listed messages
// carry. Resolved once per page (the connector holds no per-connection state)
// rather than matched by name: display names are localized and renameable, the
// well-known id is neither.
func (a *httpAPI) SentFolderID(ctx context.Context, accessToken string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	q := url.Values{paramSelect: {"id"}}
	if _, err := a.get(ctx, accessToken, a.base+"/me/mailFolders/sentitems?"+q.Encode(), nil, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		// A 2xx that names no folder is not an answer. Returning "" would make
		// the caller's `parentFolderId == sentFolder` compare two empty strings
		// and read the ABSENCE of the signal as the signal, attesting a whole
		// page — the one direction this evidence must never fail.
		return "", fmt.Errorf("graph: the mailbox reported no sent-items folder id: %w", ErrUnreachable)
	}
	return out.ID, nil
}
