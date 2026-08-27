// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The transport slice for the BYOK credentials: /ai/provider-keys.
//
// It carries nothing the store owns — no RBAC decision, no vendor vocabulary,
// no sealing. What it does own is the shape of the answer, and the one rule
// that shape has: the key has no read path. There is no GET that returns it, no
// field on the list that hints at it, and no error that echoes it back. A
// credential a client can read is one a support bundle, a browser cache and a
// screenshare each carry a copy of.

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WithProviderKeys wires the credential store. Absent it every route answers
// 503 rather than 500: a role that composed no store is an installation with
// nowhere to seal a key, which its operator can fix — see vaultUnavailable.
func (h Handlers) WithProviderKeys(store *ProviderKeyStore) Handlers {
	h.providerKeys = store
	return h
}

// ListAiProviderKeys implements (GET /ai/provider-keys).
func (h Handlers) ListAiProviderKeys(w http.ResponseWriter, r *http.Request) {
	if h.providerKeys == nil {
		vaultUnavailable(w, r, principal.ActionRead)
		return
	}
	statuses, err := h.providerKeys.List(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.AiProviderKeyList{
		Providers: make([]crmcontracts.AiProviderKeyStatus, 0, len(statuses)),
	}
	for _, s := range statuses {
		out.Providers = append(out.Providers, crmcontracts.AiProviderKeyStatus{
			Provider:   s.Provider,
			Configured: s.Configured,
			EnvVar:     s.EnvVar,
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// vaultUnavailable answers the one reason this surface can be unwired.
//
// 503, not 501. The store is nil for exactly one reason — compose builds it
// inside WithKeyvault, so a role that composes no vault has none — and that is
// an installation lacking somewhere to seal a key, which is the operator's to
// fix. 501 would say this BUILD does not implement the operation, sending an
// integrator to look for a newer version that does not exist and cannot help.
// The contract declares 503 with this meaning on all three routes.
func vaultUnavailable(w http.ResponseWriter, r *http.Request, action principal.Action) {
	// The GRANT first. Without this the wiring check answers before any
	// authorization does, and the status code becomes an oracle: a seat with no
	// `ai_routing` grant gets 503 on an installation with no vault and 403 on one
	// that has it, so anybody with a session can read off whether a vault root
	// key is configured. Same object and action the store would have gated on,
	// so a caller who IS entitled sees no difference.
	if err := auth.Require(r.Context(), providerKeysObject, action); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.ServiceUnavailable(w, r, ErrVaultUnavailable.Error())
}

// SetAiProviderKey implements (PUT /ai/provider-keys/{provider}).
//
// 204 and no body, deliberately: the only thing a caller could want back is the
// credential they just sent, and echoing it would put it in a response body
// that proxies log and browsers cache.
func (h Handlers) SetAiProviderKey(w http.ResponseWriter, r *http.Request, provider string) {
	if h.providerKeys == nil {
		vaultUnavailable(w, r, principal.ActionUpdate)
		return
	}
	var body crmcontracts.AiProviderKeyInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.providerKeys.Set(r.Context(), provider, writtenKey(body.ApiKey)); err != nil {
		// A missing vault is the operator's to fix and nothing the caller sent,
		// so it reads as unavailable rather than as their bad request.
		if errors.Is(err, ErrVaultUnavailable) {
			vaultUnavailable(w, r, principal.ActionUpdate)
			return
		}
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writtenKey reads the credential the caller sent.
//
// A pointer because the schema marks `api_key` writeOnly, which is what keeps
// the field out of every generated response type — the guarantee the
// description makes. Absent becomes empty, and the store refuses empty BY NAME
// ("remove the credential instead of storing nothing") rather than sealing a
// zero-length key that would authenticate nothing while reading as configured.
//
// Not shared with the several other nil-deref helpers this tree already carries
// (compose's `derefString` among them): a module may not import compose
// (ADR-0054 §3), so that one is unreachable from here regardless. A single
// generic home for all of them would be `shared/`, the stdlib-only leaf tier —
// worth doing as its own change across every twin, and not worth reaching for
// while adding the next one in isolation.
func writtenKey(sent *string) string {
	if sent == nil {
		return ""
	}
	return *sent
}

// DeleteAiProviderKey implements (DELETE /ai/provider-keys/{provider}).
func (h Handlers) DeleteAiProviderKey(w http.ResponseWriter, r *http.Request, provider string) {
	if h.providerKeys == nil {
		vaultUnavailable(w, r, principal.ActionUpdate)
		return
	}
	if err := h.providerKeys.Remove(r.Context(), provider); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
