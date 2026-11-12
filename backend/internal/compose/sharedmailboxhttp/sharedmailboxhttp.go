// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package sharedmailboxhttp serves the admin surface of shared mailboxes
// (/v1/shared-mailboxes). Every rule lives in capture's Registry; this
// package decodes, picks the provider's credential check and renders.
package sharedmailboxhttp

import (
	"context"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Handlers serves the shared-mailbox routes over the capture registry.
type Handlers struct {
	Registry *capture.Registry
	// ProbeIMAP runs the login check every IMAP connect runs, answering a
	// refusal on w.
	ProbeIMAP func(context.Context, http.ResponseWriter, *http.Request, *crmcontracts.ImapCredentials) (connector.Auth, bool)
	// Scopes are the scopes a provider's descriptor declares.
	Scopes func(provider string) []principal.Scope
}

// List answers every shared mailbox to its admin.
func (h Handlers) List(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		httperr.NotImplemented(w, r, "ListSharedMailboxes")
		return
	}
	boxes, err := h.Registry.SharedMailboxes(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.SharedMailboxList{Items: make([]crmcontracts.SharedMailbox, 0, len(boxes))}
	for _, box := range boxes {
		out.Items = append(out.Items, sharedMailboxOnWire(box))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// Connect connects a shared mailbox, or reconnects the stored one.
func (h Handlers) Connect(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		httperr.NotImplemented(w, r, "ConnectSharedMailbox")
		return
	}
	// Asked before the body is read, so a non-admin never makes this
	// server dial a mail host.
	actor, err := capture.RequireSharedMailboxAdmin(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.ConnectSharedMailboxRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	provider := string(req.Provider)
	// A session carries no passport scopes, so the admin lends the
	// connector the scopes its descriptor declares, as a personal connect does.
	grantor := actor
	grantor.Scopes = principal.NewScopeSet(h.Scopes(provider)...)
	ctx := principal.WithActor(r.Context(), grantor)
	r = r.WithContext(ctx)
	credential, ok := h.sharedCredential(w, r, req)
	if !ok {
		return
	}
	in := capture.SharedConnect{
		Provider: provider, Address: string(req.Address), ReaderTeam: ids.UUID(req.ReaderTeamId), Auth: credential,
	}
	if req.MailVisibility != nil {
		in.Posture = string(*req.MailVisibility)
	}
	id, err := h.Registry.ConnectShared(ctx, in)
	if err != nil {
		writeConnectError(w, r, provider, err)
		return
	}
	h.writeSharedMailbox(w, r, id)
}

// sharedCredential reads the credentials member of the provider a shared
// connect names. IMAP is checked here; graph and gmail are probed by
// ConnectShared, after the bundle is built.
func (h Handlers) sharedCredential(w http.ResponseWriter, r *http.Request, req crmcontracts.ConnectSharedMailboxRequest) (connector.Auth, bool) {
	switch req.Provider {
	case crmcontracts.ConnectSharedMailboxRequestProviderImap:
		return h.ProbeIMAP(r.Context(), w, r, req.Imap)
	case crmcontracts.ConnectSharedMailboxRequestProviderGraph:
		return graphBundle(w, r, string(req.Address), req.Graph)
	case crmcontracts.ConnectSharedMailboxRequestProviderGmail:
		return h.gmailBundle(w, r, string(req.Address), req.Gmail)
	}
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusUnprocessableEntity, Code: "connector_unsupported",
		Detail: "This provider cannot be connected as a shared mailbox.",
	})
	return nil, false
}

// Update moves a shared mailbox to other readers or another visibility.
func (h Handlers) Update(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if h.Registry == nil {
		httperr.NotImplemented(w, r, "UpdateSharedMailbox")
		return
	}
	var req crmcontracts.UpdateSharedMailboxRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	change := capture.SharedChange{Confirmed: req.Confirm, ApplyToHistory: req.ApplyToHistory != nil && *req.ApplyToHistory}
	if req.ReaderTeamId != nil {
		team := ids.UUID(*req.ReaderTeamId)
		change.ReaderTeam = &team
	}
	if req.MailVisibility != nil {
		posture := string(*req.MailVisibility)
		change.Posture = &posture
	}
	box, err := h.Registry.ChangeSharedMailbox(r.Context(), ids.UUID(id), change)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, sharedMailboxOnWire(box))
}

// Disconnect stops a shared mailbox's capture.
func (h Handlers) Disconnect(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if h.Registry == nil {
		httperr.NotImplemented(w, r, "DisconnectSharedMailbox")
		return
	}
	if err := h.Registry.DisconnectShared(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handlers) writeSharedMailbox(w http.ResponseWriter, r *http.Request, id ids.UUID) {
	box, err := h.Registry.SharedMailbox(r.Context(), id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, sharedMailboxOnWire(box))
}

func sharedMailboxOnWire(v capture.SharedMailboxView) crmcontracts.SharedMailbox {
	out := crmcontracts.SharedMailbox{
		Id: openapi_types.UUID(v.ID), Provider: crmcontracts.SharedMailboxProvider(v.Provider), Address: v.Address,
		ReaderTeamId: openapi_types.UUID(v.ReaderTeam), MailVisibility: crmcontracts.SharedMailboxMailVisibility(v.Posture),
		Status: crmcontracts.SharedMailboxStatus(v.Status), ConnectedAt: v.ConnectedAt,
	}
	if v.ConnectedBy != nil {
		by := openapi_types.UUID(*v.ConnectedBy)
		out.ConnectedBy = &by
	}
	return out
}
