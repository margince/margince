// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package sharedmailboxhttp

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// graphBundle seals the Entra app a graph connect names. ConnectShared probes it.
func graphBundle(w http.ResponseWriter, r *http.Request, address string, in *crmcontracts.SharedGraphCredentials) (connector.Auth, bool) {
	if in == nil || blank(in.TenantId) || blank(in.ClientId) || blank(in.ClientSecret) {
		writeCredentialsRequired(w, r, "graph_credentials_required", "graph", "tenant_id, client_id and client_secret")
		return nil, false
	}
	auth, err := graph.SealShared(graph.SharedCredential{
		TenantID: strings.TrimSpace(*in.TenantId), ClientID: strings.TrimSpace(*in.ClientId),
		ClientSecret: *in.ClientSecret, Mailbox: address,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return nil, false
	}
	return auth, true
}

// gmailBundle seals the service-account key a gmail connect names. The
// address is refused first when it is outside the own domains, so a foreign
// mailbox never reaches Google.
func (h Handlers) gmailBundle(w http.ResponseWriter, r *http.Request, address string, in *crmcontracts.SharedGmailCredentials) (connector.Auth, bool) {
	if in == nil || blank(in.ServiceAccountKey) {
		writeCredentialsRequired(w, r, "gmail_credentials_required", "gmail", "service_account_key")
		return nil, false
	}
	if err := h.Registry.RefuseForeignSharedMailbox(r.Context(), address); err != nil {
		writeConnectError(w, r, "gmail", err)
		return nil, false
	}
	auth, err := gmail.SealShared(gmail.SharedCredential{ServiceAccountKey: *in.ServiceAccountKey, Mailbox: address})
	if err != nil {
		httperr.Write(w, r, err)
		return nil, false
	}
	return auth, true
}

func blank(field *string) bool { return field == nil || strings.TrimSpace(*field) == "" }

func writeCredentialsRequired(w http.ResponseWriter, r *http.Request, code, provider, fields string) {
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusUnprocessableEntity, Code: code,
		Detail: "The " + provider + " provider needs " + fields + " in its credentials member.",
	})
}

// refusalDetails say what an admin fixes for each refused probe, per provider.
var refusalDetails = map[string]map[string]string{
	"graph": {
		connector.SharedCredentialsInvalid: "The tenant id, client id or mailbox address is not usable. The tenant id must be the directory id or a verified domain.",
		connector.SharedGrantRefused:       "Microsoft refused the app. Check the tenant id, client id and secret, and that an admin granted consent.",
		connector.SharedMailboxDenied:      "The app may not read this mailbox or its user record. Check the Mail.Read and User.ReadBasic.All application permissions, and the Exchange scope.",
		connector.SharedMailboxMissing:     "Microsoft knows no mailbox at this address in the tenant.",
	},
	"gmail": {
		connector.SharedCredentialsInvalid: "The service-account key or the mailbox address is not usable. Paste the whole JSON key file.",
		connector.SharedGrantRefused:       "Google refused the delegation. Check that the service account's client id holds gmail.readonly under domain-wide delegation.",
		connector.SharedMailboxDenied:      "Google refused to open this mailbox. Check that Gmail is on for this user.",
		connector.SharedMailboxMissing:     "Google knows no mailbox at this address.",
	},
}

// writeConnectError answers a refused shared connect by its class. The
// provider's own text and the secret stay out of the answer.
func writeConnectError(w http.ResponseWriter, r *http.Request, provider string, err error) {
	var refusal *connector.SharedMailboxRefusal
	var foreign *capture.ForeignMailboxError
	switch {
	case errors.As(err, &foreign):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity, Code: "shared_mailbox_foreign_domain", Detail: foreign.Error(),
		})
	case errors.As(err, &refusal):
		slog.InfoContext(r.Context(), "shared mailbox: the provider refused the probe", "provider", provider, "err", err)
		writeRefusal(w, r, refusal.Reason, refusalDetails[provider][refusal.Reason])
	case errors.Is(err, connector.ErrUnreachable) || errors.Is(err, connector.ErrRateLimited):
		slog.WarnContext(r.Context(), "shared mailbox: the provider could not be reached", "provider", provider, "err", err)
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusBadGateway, Code: "shared_mailbox_unreachable",
			Detail: "The mail provider could not be reached. Try again in a few minutes.",
		})
	default:
		httperr.Write(w, r, err)
	}
}

// writeRefusal answers a provider's refusal with the problem code of its class.
func writeRefusal(w http.ResponseWriter, r *http.Request, reason, detail string) {
	switch reason {
	case connector.SharedCredentialsInvalid:
		httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusUnprocessableEntity, Code: "shared_mailbox_credentials_invalid", Detail: detail})
	case connector.SharedGrantRefused:
		httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusUnprocessableEntity, Code: "shared_mailbox_grant_refused", Detail: detail})
	case connector.SharedMailboxMissing:
		httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusUnprocessableEntity, Code: "shared_mailbox_mailbox_not_found", Detail: detail})
	default:
		httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusUnprocessableEntity, Code: "shared_mailbox_mailbox_denied", Detail: detail})
	}
}
