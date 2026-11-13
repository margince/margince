// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/platform/googlejwt"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var errNoSharedReader = errors.New("gmail: this connector is not set up to read shared mailboxes")

// WithSharedMailboxes returns a copy that also reads shared mailboxes, with
// tokens from a service account under domain-wide delegation.
func (c *Connector) WithSharedMailboxes(cfg SharedAuthConfig) *Connector {
	copied := *c
	copied.shared = NewSharedAuthorizer(cfg)
	return &copied
}

// ProbeShared mints a delegated token for the mailbox and reads its profile.
func (c *Connector) ProbeShared(ctx context.Context, auth connector.Auth) (connector.Auth, error) {
	if c.shared == nil {
		return nil, errNoSharedReader
	}
	cred, err := ReadShared(auth)
	if err != nil {
		return nil, err
	}
	if _, err := oauthflow.MailboxAddress(cred.Mailbox); err != nil {
		return nil, &connector.SharedMailboxRefusal{Reason: connector.SharedCredentialsInvalid, Err: err}
	}
	if _, err := googlejwt.ParseServiceAccount(cred.ServiceAccountKey); err != nil {
		return nil, &connector.SharedMailboxRefusal{Reason: connector.SharedCredentialsInvalid, Err: err}
	}
	access, err := c.shared.AccessToken(ctx, auth)
	if err != nil {
		return nil, connector.SharedGrantFault(err)
	}
	if _, _, err := c.api.Profile(ctx, access); err != nil {
		return nil, connector.SharedMailboxFault(err)
	}
	return auth, nil
}

// OpenShared returns a copy that reads the sealed bundle's mailbox with its
// delegated token. The auth state it returns names the owner a sync reads.
// The API is unchanged, because the token already is the mailbox's.
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
	mailbox, err := oauthflow.MailboxAddress(cred.Mailbox)
	if err != nil {
		return nil, nil, fmt.Errorf("gmail: %w: %w", err, ErrAuthRejected)
	}
	//nolint:gosec // G117: the state carries no refresh token; the field stays empty for a shared mailbox
	state, err := json.Marshal(authState{Owner: mailbox, Scopes: scopeStrings(c.Descriptor().Scopes)})
	if err != nil {
		return nil, nil, fmt.Errorf("gmail: encoding the shared mailbox state: %w", err)
	}
	opened := *c
	opened.oauth = timedAuthorizer{delegatedToken{tokens: c.shared, bundle: auth}}
	return &opened, state, nil
}

// delegatedToken is the Authorizer of one opened shared mailbox. It ignores
// the refresh token a personal call passes, and mints from the sealed bundle.
type delegatedToken struct {
	tokens *SharedAuthorizer
	bundle connector.Auth
}

func (t delegatedToken) Exchange(context.Context, string, string) (oauthflow.TokenGrant, error) {
	return oauthflow.TokenGrant{}, fmt.Errorf("gmail: a shared mailbox is never connected by consent: %w", ErrAuthRejected)
}

func (t delegatedToken) AccessToken(ctx context.Context, _ string) (string, error) {
	return t.tokens.AccessToken(ctx, t.bundle)
}
