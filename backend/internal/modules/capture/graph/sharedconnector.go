// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sharedCheckpointMessages bounds the messages a shared round fetches between
// two checkpoints, and the page size a shared client asks for. A shared
// mailbox is large, and its app token is throttled per mailbox.
const sharedCheckpointMessages = 25

// directoryObjectID is the shape of an Entra object id.
var directoryObjectID = regexp.MustCompile(`^(?i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

var errNoSharedReader = errors.New("graph: this connector is not set up to read shared mailboxes")

// SharedMailboxConfig wires the shared-mailbox reader. Empty fields take
// Microsoft's endpoints and a bounded client; tests set them.
type SharedMailboxConfig struct {
	Auth       SharedAuthConfig
	HTTPClient *http.Client
	APIBase    string
}

type sharedReader struct {
	tokens *SharedAuthorizer
	client *http.Client
	base   string
}

// WithSharedMailboxes returns a copy that also reads shared mailboxes.
func (c *Connector) WithSharedMailboxes(cfg SharedMailboxConfig) *Connector {
	copied := *c
	copied.shared = &sharedReader{tokens: NewSharedAuthorizer(cfg.Auth), client: cfg.HTTPClient, base: cfg.APIBase}
	return &copied
}

// ProbeShared mints an app token, reads the mailbox's object id, and opens
// its inbox. The returned bundle carries the id, so a sync knows the mailbox
// by both names Graph uses for it.
func (c *Connector) ProbeShared(ctx context.Context, auth connector.Auth) (connector.Auth, error) {
	if c.shared == nil {
		return nil, errNoSharedReader
	}
	cred, err := ReadShared(auth)
	if err != nil {
		return nil, err
	}
	api, err := newSharedHTTPAPI(c.shared.client, c.shared.base, cred.Mailbox, "")
	if err == nil {
		err = cred.usable()
	}
	if err != nil {
		return nil, &connector.SharedMailboxRefusal{Reason: connector.SharedCredentialsInvalid, Err: err}
	}
	access, err := c.shared.tokens.AccessToken(ctx, auth)
	if err != nil {
		return nil, connector.SharedGrantFault(err)
	}
	if cred.UserID, err = api.userID(ctx, access); err != nil {
		return nil, connector.SharedMailboxFault(err)
	}
	if _, err := api.FolderID(ctx, access, folderInbox); err != nil {
		return nil, connector.SharedMailboxFault(err)
	}
	return SealShared(cred)
}

// userID reads the mailbox's object id, which Graph may name it by in a link.
// Reading it needs User.ReadBasic.All beside Mail.Read; a 403 refuses the connect.
func (a *httpAPI) userID(ctx context.Context, access string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	q := url.Values{paramSelect: {"id"}}
	if _, err := a.get(ctx, access, a.root()+"?"+q.Encode(), nil, &out); err != nil {
		return "", err
	}
	if !directoryObjectID.MatchString(out.ID) {
		return "", fmt.Errorf("graph: the mailbox's user record named no object id: %w", ErrUnreachable)
	}
	return out.ID, nil
}

// OpenShared returns a copy that reads the sealed bundle's mailbox with its
// app token. The auth state it returns names the owner a sync reads.
//
//nolint:ireturn // implements connector.SharedMailboxReader, whose contract returns the Connector seam
func (c *Connector) OpenShared(auth connector.Auth) (connector.Connector, connector.Auth, error) {
	if c.shared == nil {
		return nil, nil, errNoSharedReader
	}
	cred, err := ReadShared(auth)
	if err != nil {
		return nil, nil, err
	}
	api, err := newSharedHTTPAPI(c.shared.client, c.shared.base, cred.Mailbox, cred.UserID)
	if err != nil {
		return nil, nil, err
	}
	state, err := graphconn.Seal(connectorName, graphconn.AuthState{
		Owner: api.names[0], Scopes: graphconn.ScopeStrings(c.Descriptor().Scopes),
	})
	if err != nil {
		return nil, nil, err
	}
	opened := *c
	opened.oauth = appToken{tokens: c.shared.tokens, bundle: auth}
	opened.api = api
	opened.rotations = nil
	opened.checkpointMessages = sharedCheckpointMessages
	return &opened, state, nil
}

// appToken is the Authorizer of one opened shared mailbox. It ignores the
// refresh token a personal call passes, and mints from the sealed bundle.
type appToken struct {
	tokens *SharedAuthorizer
	bundle connector.Auth
}

func (t appToken) Exchange(context.Context, string, string) (oauthflow.TokenGrant, error) {
	return oauthflow.TokenGrant{}, fmt.Errorf("graph: a shared mailbox is never connected by consent: %w", ErrAuthRejected)
}

func (t appToken) AccessToken(ctx context.Context, _ string) (string, error) {
	return t.tokens.AccessToken(ctx, t.bundle)
}

func (t appToken) Refresh(ctx context.Context, _ string, _ []string) (oauthflow.TokenRefresh, error) {
	access, err := t.tokens.AccessToken(ctx, t.bundle)
	if err != nil {
		return oauthflow.TokenRefresh{}, err
	}
	return oauthflow.TokenRefresh{AccessToken: access}, nil
}

// NewSharedAPI builds the client for a shared mailbox the app reads with
// application permission. Every call goes to /users/{address}, because /me
// has no meaning for an app token. userID is the mailbox's object id, or
// empty when the app may not read it.
//
//nolint:ireturn // returns the API seam by design, as NewAPI does
func NewSharedAPI(client *http.Client, base, address, userID string) (API, error) {
	return newSharedHTTPAPI(client, base, address, userID)
}

func newSharedHTTPAPI(client *http.Client, base, address, userID string) (*httpAPI, error) {
	mailbox, err := oauthflow.MailboxAddress(address)
	if err != nil {
		return nil, fmt.Errorf("graph: %w", err)
	}
	if userID != "" && !directoryObjectID.MatchString(userID) {
		return nil, fmt.Errorf("graph: the shared mailbox's user id is not an object id: %w", ErrAuthRejected)
	}
	api := newHTTPAPI(client, base, "/users/"+url.PathEscape(mailbox))
	api.maxPageSize = sharedCheckpointMessages
	api.names = []string{mailbox}
	if userID != "" {
		api.names = append(api.names, userID)
	}
	return api, nil
}

// underMailbox reports whether a decoded link path walks this mailbox. Graph
// may spell it /users/{name}/ or /users('{name}')/, by address or object id.
func (a *httpAPI) underMailbox(path string) bool {
	base, err := url.Parse(a.base)
	if err != nil {
		return false
	}
	rest, ok := strings.CutPrefix(strings.ToLower(path), strings.ToLower(base.Path))
	if !ok {
		return false
	}
	for _, form := range a.mailboxForms() {
		if strings.HasPrefix(rest, form+"/") {
			return true
		}
	}
	return false
}

// mailboxForms are the lower-cased path roots that name this shared mailbox.
func (a *httpAPI) mailboxForms() []string {
	forms := make([]string, 0, 2*len(a.names))
	for _, name := range a.names {
		name = strings.ToLower(name)
		forms = append(forms, "/users/"+name, "/users('"+name+"')")
	}
	return forms
}
