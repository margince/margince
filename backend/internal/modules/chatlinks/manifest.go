// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The app manifest Dispact installs Margince from, as agreed with Dispact.
const (
	appName        = "Margince"
	appDescription = "Links a deal or project to a channel: a card, short updates, and summaries of the talk filed on the record."
	// eventsPath is the push receiver; the manifest names it before it serves.
	eventsPath = "/v1/channel-events/dispact"
	// appTokenTTL is Dispact's default session token lifetime; Margince opens
	// no session in Dispact, and the MCP call tokens have their own.
	appTokenTTL = 3600
)

var (
	appScopes = []string{
		"chat:write:channel", "channel:create", "channel:members:write",
		"users:read:email", "channel:history:read", "channel:files:read",
	}
	// appEventTypes are the notices the app subscribes to.
	appEventTypes = []string{"message.created", "thread.replied", "message.updated", "channel.archived"}
)

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
	publisher, err := h.publisher(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, AppManifest(base, publisher))
}

// AppManifest is the manifest for an installation served at base, published
// by the admin at publisherEmail. Dispact refuses one without that address.
func AppManifest(base, publisherEmail string) crmcontracts.DispactAppManifest {
	version := buildinfo.ReleaseVersion
	if version == "" {
		version = "dev"
	}
	return crmcontracts.DispactAppManifest{
		ManifestVersion: crmcontracts.DispactAppManifestManifestVersionN1,
		Slug:            crmcontracts.DispactAppManifestSlugMargince,
		Name:            appName,
		Description:     appDescription,
		IconUrl:         base + "/icon.svg",
		Publisher:       crmcontracts.DispactAppManifestPublisher{Name: appName, Email: publisherEmail},
		Version:         version,
		Surfaces:        map[string]any{},
		Bot:             crmcontracts.DispactAppManifestBot{DisplayName: appName},
		Scopes:          appScopes,
		Security:        crmcontracts.DispactAppManifestSecurity{TokenTtlSeconds: appTokenTTL},
		Mcp:             crmcontracts.DispactAppManifestMcp{Endpoint: base + "/mcp"},
		Events:          crmcontracts.DispactAppManifestEvents{Url: base + eventsPath, Types: appEventTypes},
	}
}
