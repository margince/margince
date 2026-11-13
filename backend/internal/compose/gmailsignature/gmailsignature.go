// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package gmailsignature reads the signature a user's connected Gmail mailbox
// holds, for the settings dialog that offers to use it.
package gmailsignature

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/signature"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// gmailProvider is the capture provider name of a Gmail connection.
const gmailProvider = "gmail"

// Source resolves a user's live connection for a signature read;
// capture.Registry is the production one.
type Source interface {
	SignatureReaderFor(ctx context.Context, userID ids.UserID, provider string) (connector.SendAsSignatureReader, connector.Auth, []string, error)
}

// Read answers the user's Gmail send-as signature, cleaned the way a saved
// personal signature is. It writes nothing; a nil source has no mailboxes.
func Read(ctx context.Context, source Source, user ids.UserID) (crmcontracts.GmailSignatureImport, error) {
	state := func(s crmcontracts.GmailSignatureImportState) crmcontracts.GmailSignatureImport {
		return crmcontracts.GmailSignatureImport{State: s}
	}
	if source == nil {
		return state(crmcontracts.GmailSignatureImportStateNoMailbox), nil
	}
	reader, auth, granted, err := source.SignatureReaderFor(ctx, user, gmailProvider)
	switch {
	case errors.Is(err, capture.ErrNoConnection), errors.Is(err, capture.ErrConnectorNotConfigured),
		errors.Is(err, capture.ErrConnectorHasNoSignature):
		return state(crmcontracts.GmailSignatureImportStateNoMailbox), nil
	case err != nil:
		return crmcontracts.GmailSignatureImport{}, err
	case !gmail.CanReadSendAs(granted):
		return state(crmcontracts.GmailSignatureImportStateNeedsReconnect), nil
	}
	held, err := reader.SendAsSignature(ctx, auth)
	if errors.Is(err, connector.ErrAuthRejected) {
		return state(crmcontracts.GmailSignatureImportStateNeedsReconnect), nil
	}
	if err != nil {
		return crmcontracts.GmailSignatureImport{}, err
	}
	markup, text, err := activities.SignatureMarkup(held.HTML)
	if err != nil {
		return crmcontracts.GmailSignatureImport{}, err
	}
	markup = strings.TrimSpace(markup)
	out := crmcontracts.GmailSignatureImport{State: crmcontracts.GmailSignatureImportStateReady}
	if held.Address != "" {
		out.Address = &held.Address
	}
	switch {
	// A send skips personal HTML with no words in it (activities/signature.go),
	// so an image-only signature is answered as one there is nothing to use of.
	case markup == "" || strings.TrimSpace(text) == "":
		out.State = crmcontracts.GmailSignatureImportStateEmpty
	case utf8.RuneCountInString(markup) > signature.MaxHTMLRunes:
		out.State = crmcontracts.GmailSignatureImportStateTooLong
	default:
		out.Html, out.Text = &markup, &text
	}
	return out, nil
}

// ImportHandlers serves the contract's read of the caller's Gmail signature.
type ImportHandlers struct {
	// Source is nil in a role with no capture registry, which has no mailboxes.
	Source Source
}

// GetMyGmailSignature reads the caller's Gmail signature on their press. It
// writes nothing: the settings dialog shows it, and Save stores it.
func (h ImportHandlers) GetMyGmailSignature(w http.ResponseWriter, r *http.Request) {
	actor, ok := principal.Actor(r.Context())
	if !ok || actor.Type != principal.PrincipalHuman {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	out, err := Read(r.Context(), h.Source, ids.From[ids.UserKind](actor.UserID))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
