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
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/testmailbox"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// hasConnector reports whether name is registered on this role's capture
// registry — the same fact NewCaptureRegistry gated test_mailbox's
// registration on, asked directly rather than duplicated as a second flag
// that could drift from it.
func (h connectorHandlers) hasConnector(name string) bool {
	for _, d := range h.registry.Connectors() {
		if d.Name == name {
			return true
		}
	}
	return false
}

// connectTestMailbox mints an opaque connection: the "credential" is just
// the granting human's own user_id, known BEFORE Registry.Connect ever runs
// — unlike the connection ROW's own id (which upsertConnection's SQL
// generates), there is no chicken-and-egg here, because
// capture.TestMailboxLedger keys its bookkeeping on user_id, not on a
// specific connection row. There is nothing here that carries any real
// secret, but Registry.Connect still vault-seals it like any other
// connector's credential, which is what the disconnect/re-connect lifecycle
// already handles generically.
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
	grantor.Scopes = principal.NewScopeSet(testmailbox.New(nil).Descriptor().Scopes...)
	ctx := principal.WithActor(r.Context(), grantor)

	// The wire shape (`{"user_id": "<uuid>"}`) matches testmailbox's own
	// unexported authPayload by convention rather than a shared type —
	// offlinedemo's authPayload is unexported the same way, and its own
	// writer (scripts/seed-dev.sql) constructs the equivalent JSON
	// independently for the same reason: the shape is the connector's
	// public contract with whatever mints its Auth, not a Go type to share.
	auth, err := json.Marshal(struct {
		UserID string `json:"user_id"`
	}{UserID: actor.UserID.String()})
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
