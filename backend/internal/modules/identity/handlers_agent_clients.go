// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The /installation/agent-clients operations: the admin's view of the OAuth
// clients AI tools sign users in through. The store gates every one.

import (
	"errors"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListAgentClients implements (GET /installation/agent-clients).
func (h Handlers) ListAgentClients(w http.ResponseWriter, r *http.Request, params crmcontracts.ListAgentClientsParams) {
	actor, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "MCP clients are managed by a signed-in human")
		return
	}
	clients, page, err := h.svc.ListAgentClients(r.Context(), actor, params.Cursor, params.Limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The reader's gate already passed above; the switches are read as the
	// system so this page and a sign-in can never see different values.
	state, err := h.registrationState(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	data := make([]crmcontracts.AgentClient, 0, len(clients))
	for _, c := range clients {
		data = append(data, wireAgentClient(c))
	}
	effective := state.effective()
	w.Header().Set("Cache-Control", "private, no-store")
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AgentClientListResponse{
		Data: data,
		Page: pageInfo(page),
		Registration: crmcontracts.AgentClientRegistration{
			Dynamic: crmcontracts.AgentClientMethod{
				Enabled: effective.Dynamic, Setting: state.switches.Dynamic, DeploymentAllows: state.deployment.Dynamic,
			},
			MetadataDocument: crmcontracts.AgentClientMethod{
				Enabled: effective.MetadataDocument, Setting: state.switches.MetadataDocument,
				DeploymentAllows: state.deployment.MetadataDocument,
			},
		},
	})
}

// RegisterAgentClient implements (POST /installation/agent-clients).
func (h Handlers) RegisterAgentClient(w http.ResponseWriter, r *http.Request) {
	actor, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "MCP clients are registered by a signed-in human")
		return
	}
	var req crmcontracts.RegisterAgentClientRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	client, secret, err := h.svc.RegisterAgentClient(r.Context(), actor, RegisterAgentClientInput{
		Name: req.Name, RedirectURIs: req.RedirectUris,
	})
	if err != nil {
		writeAgentClientError(w, r, err)
		return
	}
	body := crmcontracts.RegisteredAgentClient{
		Client:       wireAgentClient(client),
		Scopes:       agentClientScopes(),
		ClientSecret: secret,
	}
	w.Header().Set("Cache-Control", "no-store")
	httperr.WriteJSON(w, http.StatusCreated, body)
}

// UpdateAgentClient implements (PATCH /installation/agent-clients/{id}).
func (h Handlers) UpdateAgentClient(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	actor, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "MCP clients are managed by a signed-in human")
		return
	}
	// A pointer, so a body without disabled is refused rather than read as
	// false, which would turn a disabled client back on.
	var req struct {
		Disabled *bool `json:"disabled"`
	}
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Disabled == nil {
		httperr.Write(w, r, httperr.Validation("disabled", "required", "say whether the client is disabled"))
		return
	}
	client, err := h.svc.SetAgentClientDisabled(r.Context(), actor, ids.UUID(id), *req.Disabled)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireAgentClient(client))
}

// DeleteAgentClient implements (DELETE /installation/agent-clients/{id}).
func (h Handlers) DeleteAgentClient(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	actor, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "MCP clients are managed by a signed-in human")
		return
	}
	if err := h.svc.DeleteAgentClient(r.Context(), actor, ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RotateAgentClientSecret implements (POST /installation/agent-clients/{id}/secret).
func (h Handlers) RotateAgentClientSecret(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	actor, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "MCP clients are managed by a signed-in human")
		return
	}
	secret, err := h.svc.RotateAgentClientSecret(r.Context(), actor, ids.UUID(id))
	if err != nil {
		writeAgentClientError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AgentClientSecret{ClientSecret: secret, Scopes: agentClientScopes()})
}

func writeAgentClientError(w http.ResponseWriter, r *http.Request, err error) {
	if bad, ok := errors.AsType[*InvalidAgentClientFieldError](err); ok {
		httperr.Write(w, r, httperr.Validation(bad.Field, bad.Code, bad.Message))
		return
	}
	httperr.Write(w, r, err)
}

func wireAgentClient(c AgentClient) crmcontracts.AgentClient {
	return crmcontracts.AgentClient{
		Id:             openapi_types.UUID(c.ID),
		ClientId:       c.ClientID,
		ClientName:     c.ClientName,
		RedirectUris:   c.RedirectURIs,
		RegisteredBy:   crmcontracts.AgentClientRegisteredBy(c.RegisteredBy),
		Confidential:   c.Confidential,
		Disabled:       c.Disabled,
		CreatedAt:      c.CreatedAt,
		ConnectedUsers: c.ConnectedUsers,
		LastUsedAt:     c.LastUsedAt,
	}
}

// agentClientScopes is what an admin pastes into a tool: the scopes discovery
// advertises, offline_access included.
func agentClientScopes() string {
	return strings.Join(oauthScopesSupported, " ")
}
