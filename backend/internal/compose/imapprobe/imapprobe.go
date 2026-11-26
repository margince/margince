// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package imapprobe is the login check every IMAP connect runs, a personal
// mailbox's or a shared one's.
package imapprobe

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/imap"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Authenticate dials the server and logs in; it answers the connector's
// sentinels for a rejected login or an unreachable server.
type Authenticate func(context.Context, connector.AuthRequest) (connector.Auth, error)

// Probe turns IMAP credentials into a stored credential with the login check
// every IMAP connect runs. A refusal is answered on w.
func Probe(ctx context.Context, w http.ResponseWriter, r *http.Request, in *crmcontracts.ImapCredentials, authenticate Authenticate) (connector.Auth, bool) {
	if in == nil || in.Secret == nil || in.Host == "" || in.Username == "" {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity,
			Code:   "imap_credentials_required",
			Detail: "The imap provider needs host, username and secret in the request body.",
		})
		return nil, false
	}
	creds := imap.Credentials{Host: in.Host, Email: in.Username, Password: *in.Secret}
	// Port, Mailbox and MaxMessages stay zero when absent, so the
	// connector's own defaults and caps apply instead of a copied zero.
	if in.Port != nil {
		creds.Port = *in.Port
	}
	if in.Mailbox != nil {
		creds.Mailbox = *in.Mailbox
	}
	if in.MaxMessages != nil {
		creds.MaxMessages = *in.MaxMessages
	}
	creds.SMTP = smtpSettings(in.Smtp)
	authReq, err := imap.AuthRequestFrom(creds)
	if err != nil {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity,
			Code:   "imap_credentials_invalid",
			Detail: "These credentials could not be processed.",
		})
		return nil, false
	}
	auth, err := authenticate(ctx, authReq)
	if err != nil {
		writeConnectError(w, r, err)
		return nil, false
	}
	return auth, true
}

// smtpSettings turns sending on when the request carries settings for it.
// Absent fields stay zero, so the connector's own defaults apply.
func smtpSettings(in *crmcontracts.ImapSmtpSettings) *imap.SMTPSettings {
	if in == nil {
		return nil
	}
	out := &imap.SMTPSettings{}
	if in.Host != nil {
		out.Host = *in.Host
	}
	if in.Port != nil {
		out.Port = *in.Port
	}
	if in.Security != nil {
		out.Security = string(*in.Security)
	}
	return out
}

// writeConnectError maps the connector sentinels onto the transport
// without leaking the provider's raw error.
func writeConnectError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, imap.ErrSendAddressInvalid):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity,
			Code:   "smtp_address_invalid",
			Detail: "To send from this mailbox, its login must be an email address.",
		})
	case errors.Is(err, imap.ErrSendLoginRejected):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity,
			Code:   "smtp_login_rejected",
			Detail: "The outgoing mail server rejected these credentials. Check the SMTP host, port, security and app password.",
		})
	case errors.Is(err, imap.ErrSendUnreachable):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusBadGateway,
			Code:   "smtp_unreachable",
			Detail: "The outgoing mail server could not be reached.",
		})
	case errors.Is(err, imap.ErrLoginRejected):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity,
			Code:   "imap_login_rejected",
			Detail: "The mailbox rejected these credentials. Check host, email and app password.",
		})
	case errors.Is(err, imap.ErrUnreachable):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusBadGateway,
			Code:   "imap_unreachable",
			Detail: "The mail server could not be reached.",
		})
	default:
		slog.ErrorContext(r.Context(), "imap connector: authenticate", "err", err)
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError,
			Code:   "imap_connect_failed",
			Detail: "The connection could not be established.",
		})
	}
}
