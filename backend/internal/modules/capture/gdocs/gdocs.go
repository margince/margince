// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package gdocs is the Google Docs connection: a user's own grant to read the
// meeting notes and transcripts Google Meet writes into Google Docs.
//
// It is its own connection with its own consent, beside Gmail and Google
// Calendar on the same Google app. The calendar connection keeps its scopes.
// The grant is documents.readonly: it opens a Doc by its id and cannot list or
// search Drive. The address scope names the account that connected.
//
// The connection writes no records of its own. Its Sync mints an access token,
// so a revoked grant shows as "reconnect" on Connections. It then reads the
// Docs the Sink names and hands their text back to it.
package gdocs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const connectorName = "gdocs"

// ReadScope reads one Google Doc by its id. It is the scope that decides
// whether this connection may read meeting notes at all.
const ReadScope = "https://www.googleapis.com/auth/documents.readonly"

// EmailScope lets the consent read the account's address, which names the
// connection. documents.readonly has no call that returns it.
const EmailScope = "https://www.googleapis.com/auth/userinfo.email"

// ConsentScopes are what a Google Docs consent asks Google for.
func ConsentScopes() []string {
	return []string{ReadScope, EmailScope}
}

// Default Google OAuth endpoints; overridable via OAuthConfig for tests.
const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token" //nolint:gosec // G101 false positive: Google's public OAuth token endpoint URL, not a credential
)

// OAuth is the shared Google handshake surface.
type OAuth = googleconn.OAuth

// OAuthConfig wires the Docs OAuth client. AuthURL and TokenURL default to
// Google's endpoints when empty; tests override them.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
}

// NewOAuth builds the Docs OAuth client on the shared oauthflow handshake. It
// leaves out include_granted_scopes, as the calendar client does, so a grant
// made after a Gmail one never gathers the mail scopes into this credential.
//
//nolint:ireturn // returns the OAuth seam by design, so tests substitute a stub
func NewOAuth(cfg OAuthConfig) OAuth {
	if cfg.AuthURL == "" {
		cfg.AuthURL = googleAuthURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = googleTokenURL
	}
	return oauthflow.New(oauthflow.Config{
		Provider:     connectorName,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       ConsentScopes(),
		AuthURL:      cfg.AuthURL,
		TokenURL:     cfg.TokenURL,
		// Offline access and a forced consent prompt make Google return a
		// refresh token.
		AuthParams: map[string]string{
			"access_type": "offline",
			"prompt":      "consent",
		},
		AuthRejected: googleconn.ErrAuthRejected,
		Unreachable:  googleconn.ErrUnreachable,
	})
}

// Owner names the account behind an access token.
type Owner interface {
	Email(ctx context.Context, accessToken string) (string, error)
}

// NewOwner reads the address from Google's userinfo endpoint over client and
// base, Google's when empty.
//
//nolint:ireturn // returns the Owner seam by design, so tests substitute a stub
func NewOwner(client *http.Client, base string) Owner {
	return googleconn.NewUserinfo(client, base)
}

// Connector authorizes a Google Docs connection. It holds no per-connection
// state; one instance is registered and shared.
type Connector struct {
	oauth googleconn.Authorizer
	owner Owner
	docs  DocsAPI
}

// New returns a Docs connector over the given OAuth and owner surfaces.
func New(oauth googleconn.Authorizer, owner Owner) *Connector {
	return &Connector{oauth: oauth, owner: owner}
}

var (
	_ connector.Connector      = (*Connector)(nil)
	_ connector.GrantedScoper  = (*Connector)(nil)
	_ connector.AccountLabeler = (*Connector)(nil)
)

// WithDocs returns a copy that reads meeting notes through docs. Without it a
// sync only refreshes the token.
func (c *Connector) WithDocs(docs DocsAPI) *Connector {
	copied := *c
	copied.docs = docs
	return &copied
}

// errNoRecords marks the one record shape this connector never produces.
var errNoRecords = errors.New("gdocs: the Google Docs connection captures no records")

// AuthRequestFrom packages an OAuth callback's code into the connector's
// AuthRequest.
func AuthRequestFrom(code, redirectURI string) (connector.AuthRequest, error) {
	return googleconn.AuthRequestFrom(code, redirectURI)
}

// Descriptor is the shared read-only Google connector shape under "gdocs".
func (c *Connector) Descriptor() connector.Descriptor {
	return googleconn.Descriptor(connectorName)
}

// Authenticate exchanges the code for a refresh token and names the account
// from its address.
func (c *Connector) Authenticate(ctx context.Context, req connector.AuthRequest) (connector.Auth, error) {
	return googleconn.Authenticate(ctx, c.oauth, req, c.Descriptor().Scopes, c.owner.Email)
}

// GrantedScopes reports the Google scopes this connection holds.
func (c *Connector) GrantedScopes(auth connector.Auth) ([]string, error) {
	return googleconn.GrantedScopes(auth)
}

// AccountLabel reports the Google account this connection authorized.
func (c *Connector) AccountLabel(auth connector.Auth) (string, error) {
	return googleconn.AccountLabel(auth)
}

// Sync mints an access token, so a revoked or expired grant parks the
// connection, then reads the meeting notes the sink names. The cursor goes back
// unchanged: what is due is the sink's answer, not a position in a feed.
func (c *Connector) Sync(ctx context.Context, auth connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	_, token, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return nil, err
	}
	keeper, ok := sink.(connector.MeetingNotesKeeper)
	if !ok || c.docs == nil {
		return cursor, nil
	}
	if err := c.readMeetingNotes(ctx, keeper, token); err != nil {
		return nil, err
	}
	return cursor, nil
}

// readMeetingNotes reads each due Doc and lands its text. A Doc this account
// cannot open is skipped, and the next sync asks again. A refused grant stops
// the pass, because no other Doc can be read either.
func (c *Connector) readMeetingNotes(ctx context.Context, keeper connector.MeetingNotesKeeper, token string) error {
	due, err := keeper.MeetingNotesDue(ctx)
	if err != nil {
		return err
	}
	for _, doc := range due {
		read, err := c.docs.Document(ctx, token, doc.FileID)
		switch {
		case errors.Is(err, ErrScopeRefused):
			return keeper.MeetingNotesRefused(ctx)
		case errors.Is(err, ErrDocumentUnreadable):
			slog.InfoContext(ctx, "gdocs: a meeting's notes document could not be opened by this account",
				"meeting_id", doc.Meeting.String(), "error", err)
			continue
		case err != nil:
			return err
		}
		notes := connector.MeetingNotesText{Title: read.Title, Revision: read.Revision, Text: read.Text}
		if err := keeper.KeepMeetingNotes(ctx, doc, notes); err != nil {
			return err
		}
	}
	return nil
}

// Normalize has nothing to map: this connection writes no records.
func (c *Connector) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, errNoRecords
}

// HealthCheck confirms the stored credential still mints a token.
func (c *Connector) HealthCheck(ctx context.Context, auth connector.Auth) error {
	_, _, err := googleconn.Session(ctx, c.oauth, auth)
	return err
}
