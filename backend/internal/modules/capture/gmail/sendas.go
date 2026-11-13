// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sendAsReadScopes are the scopes Google accepts for users.settings.sendAs.list.
// The connection asks for gmail.readonly, so an ordinary grant holds one.
var sendAsReadScopes = []string{
	sharedScope,
	"https://www.googleapis.com/auth/gmail.modify",
	"https://www.googleapis.com/auth/gmail.settings.basic",
	"https://mail.google.com/",
}

// ErrSendAsScopeMissing is a connection whose grant cannot read the send-as
// settings; the user has to connect the mailbox again.
var ErrSendAsScopeMissing = errors.New("gmail: this connection was not granted a scope that reads the send-as settings")

// CanReadSendAs reports whether a grant's scopes reach the send-as settings.
func CanReadSendAs(granted []string) bool {
	return slices.ContainsFunc(sendAsReadScopes, func(scope string) bool { return slices.Contains(granted, scope) })
}

var _ connector.SendAsSignatureReader = (*Connector)(nil)

// sendAsLister is the part of the HTTP client that reads the send-as list.
// It is asserted rather than added to API, so the sync's test doubles need
// not grow a method the sync never calls.
type sendAsLister interface {
	SendAs(ctx context.Context, accessToken string) ([]sendAsEntry, error)
}

type sendAsEntry struct {
	SendAsEmail string `json:"sendAsEmail"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	Signature   string `json:"signature"`
	IsPrimary   bool   `json:"isPrimary"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	IsDefault   bool   `json:"isDefault"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
}

// SendAs lists the mailbox's send-as addresses with their signatures.
func (a *httpAPI) SendAs(ctx context.Context, accessToken string) ([]sendAsEntry, error) {
	var out struct {
		SendAs []sendAsEntry `json:"sendAs"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	}
	if _, err := a.get(ctx, accessToken, "/settings/sendAs", nil, &out, maxJSONResponseBytes); err != nil {
		return nil, err
	}
	return out.SendAs, nil
}

// SendAsSignature answers the signature of the address Gmail sends from by
// default, else of the primary address. Gmail keeps one per send-as address.
func (c *Connector) SendAsSignature(ctx context.Context, auth connector.Auth) (connector.SendAsSignature, error) {
	var st authState
	if err := json.Unmarshal(auth, &st); err != nil {
		return connector.SendAsSignature{}, fmt.Errorf("gmail: malformed auth state: %w", err)
	}
	lister, ok := c.api.(sendAsLister)
	if !ok {
		return connector.SendAsSignature{}, errors.New("gmail: this client cannot read the send-as settings")
	}
	ctx, access, err := c.authorize(ctx, st)
	if err != nil {
		return connector.SendAsSignature{}, err
	}
	entries, err := lister.SendAs(ctx, access)
	if err != nil {
		return connector.SendAsSignature{}, err
	}
	return defaultSendAs(entries), nil
}

func defaultSendAs(entries []sendAsEntry) connector.SendAsSignature {
	chosen := slices.IndexFunc(entries, func(e sendAsEntry) bool { return e.IsDefault })
	if chosen < 0 {
		chosen = slices.IndexFunc(entries, func(e sendAsEntry) bool { return e.IsPrimary })
	}
	if chosen < 0 {
		return connector.SendAsSignature{}
	}
	return connector.SendAsSignature{Address: entries[chosen].SendAsEmail, HTML: entries[chosen].Signature}
}
