// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The app manifest Dispact installs Margince from, as agreed with Dispact.
const (
	appName   = "Margince"
	appScopes = "chat:write:channel channel:create channel:members:write users:read:email channel:history:read channel:files:read"
	// eventsPath is the push receiver; the manifest names it before it serves.
	eventsPath = "/v1/channel-events/dispact"
)

// appEventTypes are the notices the app subscribes to.
var appEventTypes = []string{"message.created", "thread.replied", "message.updated", "channel.archived"}

// botConnectObject is the RBAC object that admits connecting a chat bot. The
// manifest is the other half of that install, so the same grant reads it.
const botConnectObject = "channel_connection"

// WithManifestBase serves the Dispact app manifest from the installation's
// public_base_url, read by base on each request.
func (h Handlers) WithManifestBase(base func() string) Handlers {
	h.manifestBase = base
	return h
}

// GetDispactAppManifest answers the manifest an admin pastes into Dispact.
func (h Handlers) GetDispactAppManifest(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r.Context(), botConnectObject, principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	base := ""
	if h.manifestBase != nil {
		base = strings.TrimRight(h.manifestBase(), "/")
	}
	if base == "" {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusServiceUnavailable,
			Code:   "channel_links_not_configured",
			Detail: "This deployment has no public base URL, so Dispact has no address to call.",
		})
		return
	}
	httperr.WriteJSON(w, http.StatusOK, AppManifest(base))
}

// AppManifest is the manifest for an installation served at base.
func AppManifest(base string) crmcontracts.DispactAppManifest {
	return crmcontracts.DispactAppManifest{
		Slug:   crmcontracts.DispactAppManifestSlugMargince,
		Name:   appName,
		Mcp:    crmcontracts.DispactAppManifestMcp{Endpoint: base + "/mcp"},
		Scopes: appScopes,
		Events: crmcontracts.DispactAppManifestEvents{Url: base + eventsPath, Types: appEventTypes},
	}
}
