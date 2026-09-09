// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The test_mailbox connect transport (POST /v1/connectors/test_mailbox/connect):
// no OAuth ceremony, no credential probe — there is nothing real to probe —
// just a fresh connection a QC suite can create and tear down at will.
// Reached only when test_mailbox is registered on the capture registry
// (NewCaptureRegistry, compose/capture.go), which is itself gated on
// deployconfig.Operations.AllowTestMailbox; see ConnectConnector's dispatch
// in connectors.go for the 422 otherwise.

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/testmailbox"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// codeConnectorUnsupported marks a provider this transport does not (or, for
// test_mailbox with the flag unset, currently cannot) speak for at all. Also
// used by connectors.go's own OAuth-provider branch and
// backfilltransport.go — kept here rather than there since connectors.go
// sits at this repo's 500-line file ceiling.
const codeConnectorUnsupported = "connector_unsupported"

// dispatchTestMailboxConnect handles ConnectConnector's test_mailbox branch,
// reporting whether it did — false means provider was something else and
// ConnectConnector's own dispatch continues. Kept in this file (not inlined
// in connectors.go) so that file stays under this repo's 500-line ceiling.
func (h connectorHandlers) dispatchTestMailboxConnect(w http.ResponseWriter, r *http.Request, provider string) bool {
	if provider != testmailbox.Name {
		return false
	}
	// No deployment-flag field of its own here: whether test_mailbox may be
	// connected rides the SAME fact that gated its registration
	// (NewCaptureRegistry, compose/capture.go) — asking the registry
	// directly means there is exactly one place AllowTestMailbox is read,
	// not a second copy that could drift from it.
	if h.registry == nil || !h.hasConnector(testmailbox.Name) {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity,
			Code:   codeConnectorUnsupported,
			Detail: "Only the " + strings.Join(oauthProviders, ", ") + " and imap connectors can be connected here.",
		})
		return true
	}
	h.connectTestMailbox(w, r)
	return true
}

// connectTestMailbox mints an opaque connection: the credential is just the
// granting human's own user_id (testmailbox.Credential), which
// capture.TestMailboxLedger keys its bookkeeping on. There is nothing here
// that carries any real secret, but Registry.Connect still vault-seals it
// like any other connector's credential, which is what the disconnect/
// re-connect lifecycle already handles generically.
func (h connectorHandlers) connectTestMailbox(w http.ResponseWriter, r *http.Request) {
	actor, ok := principal.Actor(r.Context())
	_, hasWS := principal.WorkspaceID(r.Context())
	if !ok || actor.Type != principal.PrincipalHuman || !hasWS {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnauthorized,
			Code:   codeUnauthorized,
			Detail: "Connecting a mailbox is a signed-in human action.",
		})
		return
	}
	// Same reasoning as connectIMAP: a cookie-session human carries no
	// passport Scopes, so the granting human must be given the connector's
	// declared scopes explicitly, from the descriptor itself so grant and
	// requirement stay coupled at one source.
	grantor := actor
	grantor.Scopes = principal.NewScopeSet(h.connectorDescriptor(testmailbox.Name).Scopes...)
	ctx := principal.WithActor(r.Context(), grantor)

	auth, err := testmailbox.Credential(actor.UserID)
	if err != nil {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError,
			Code:   codeConnectorStoreFailed,
			Detail: "The connection could not be prepared.",
		})
		return
	}
	if _, err := h.registry.Connect(ctx, testmailbox.Name, auth); err != nil {
		if errors.Is(err, apperrors.ErrScopeExceeded) {
			// Defense-in-depth: connectTestMailbox grants the descriptor's
			// scopes from the human's authority, so a human cannot normally
			// trip this.
			httperr.Write(w, r, &httperr.DetailedError{
				Status: http.StatusForbidden,
				Code:   "scope_exceeded",
				Detail: "This connection requires a capture scope that was not granted.",
			})
			return
		}
		slog.ErrorContext(r.Context(), "test_mailbox connector: persisting connection", "err", err)
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError,
			Code:   codeConnectorStoreFailed,
			Detail: "The connection could not be stored. Nothing was captured; try again.",
		})
		return
	}
	views, err := h.registry.Connections(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "test_mailbox connector: reading back connection", "err", err)
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError,
			Code:   codeConnectorStoreFailed,
			Detail: "The connection was stored but could not be read back.",
		})
		return
	}
	for _, v := range views {
		if v.Provider == testmailbox.Name {
			conn := toContractConnection(v)
			httperr.WriteJSON(w, http.StatusOK, crmcontracts.ConnectConnectorResponse{
				Connection: &conn,
			})
			return
		}
	}
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusInternalServerError,
		Code:   codeConnectorStoreFailed,
		Detail: "The connection was stored but did not appear in the read-back.",
	})
}
