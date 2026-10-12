// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// GET /agent-bundle, the Margince skill. agentAPIOrigin.baseFor is the one answer to where
// a passport calls, so the ZIP and the Settings card's snippet name one host.

import (
	"log/slog"
	"net/http"

	"github.com/margince/margince/backend/internal/compose/agentbundle"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// agentAPIOrigin is the deployment's two configured bases, and the proxies
// whose X-Forwarded-Proto the request fallback believes.
type agentAPIOrigin struct {
	api, public string
	proxies     httpserver.TrustedProxies
}

// baseFor is the API address a passport calls, `/v1` included.
func (o agentAPIOrigin) baseFor(r *http.Request) string {
	base := apiOrigin(o.api, o.public)
	if base == "" {
		// A browser cannot send a forged Host with the victim's cookie, and no-store keeps a shared cache from replaying one.
		base = requestOrigin(r, o.proxies)
	}
	return apiV1Base(base)
}

// apiOrigin is where the API is served: its own base when it has one. Else it is
// the public one, which serves the API too on a same-origin deployment.
func apiOrigin(apiBaseURL, publicBaseURL string) string {
	if apiBaseURL != "" {
		return apiBaseURL
	}
	return publicBaseURL
}

// requestOrigin is the scheme and host this request arrived on. Only a
// trusted proxy may say the scheme was https before it reached us.
func requestOrigin(r *http.Request, proxies httpserver.TrustedProxies) string {
	scheme := schemeHTTP
	if r.TLS != nil || (proxies.FromTrustedPeer(r) && r.Header.Get("X-Forwarded-Proto") == schemeHTTPS) {
		scheme = schemeHTTPS
	}
	return scheme + "://" + r.Host
}

type agentBundleHandlers struct {
	origin  agentAPIOrigin
	builder *agentbundle.Builder
	log     *slog.Logger
}

func newAgentBundleHandlers(origin agentAPIOrigin, log *slog.Logger) agentBundleHandlers {
	return agentBundleHandlers{origin: origin, builder: &agentbundle.Builder{}, log: log}
}

// DownloadAgentSkillBundle answers the skill ZIP. It is human-only here as well
// as at the gate, so the refusal does not rest on route resolution alone.
func (h agentBundleHandlers) DownloadAgentSkillBundle(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	archive, err := h.builder.Build(h.origin.baseFor(r))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httperr.Download{ContentType: "application/zip", Filename: "margince-skill.zip", Size: int64(len(archive))}.WriteHeaders(w)
	if _, err := w.Write(archive); err != nil { // #nosec G705 -- a ZIP attachment, never rendered as a page
		h.log.WarnContext(r.Context(), "agent bundle: the client's download is truncated", "err", err)
	}
}
