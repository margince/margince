// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The session-less inbound edge: how a unit's declared InboundEndpoint becomes
// a mounted route, and what the core does to a request before the unit sees it.
//
// The division is the point. The CORE resolves the installation's workspace,
// refuses a slug nothing declared, caps the body, meters both buckets and
// bounds the timestamp — everything decidable without the unit's secret. The
// UNIT verifies the signature and decides what the payload means, because the
// secret lives in its own namespace and the core has no way to read it.

package compose

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// inboundPrefix is the mounted path's fixed head. It sits beside the provider
// push receivers rather than under /v1/, because /v1/ carries the session
// middleware and this edge has no session — and because ServeMux prefers the
// longest match, a /v1/ registration here would win over the entry that carries
// that middleware.
const inboundPrefix = "/webhooks/ext/"

// InboundRoute is one mounted anonymous edge, reported so the parity sweep can
// see it.
//
// Reported rather than merely mounted, and that is deliberate: extparity's
// documented residual is that a mux.Handle nobody returned is invisible to the
// sweep. An anonymous edge is the last route in this product that should be
// invisible to a census of what is reachable.
type InboundRoute struct {
	Pattern  string
	Unit     extension.Name
	Endpoint extension.InboundEndpoint
}

// inboundUnit is one unit's declared edges, keyed for resolution.
type inboundUnit struct {
	name      extension.Name
	version   string
	endpoints map[string]extension.InboundEndpoint
}

// workspaceResolver answers the installation's one workspace. It is the
// identity service's InstallationWorkspace, taken as a function so this file
// does not reach across a module boundary for one call.
type workspaceResolver func(context.Context) (ids.WorkspaceID, error)

// MountInboundEndpoints mounts one route per declared inbound endpoint and
// reports what it registered.
//
// One handler serves every unit's edges rather than one per endpoint: the
// admission sequence is identical for all of them, and a per-endpoint closure
// would be the same sequence copied once per declaration, free to drift.
func MountInboundEndpoints(
	mux *http.ServeMux,
	exts []extension.Extension,
	resolve workspaceResolver,
	deps extensionRuntimeBinding,
	log *slog.Logger,
) []InboundRoute {
	units := make(map[string]inboundUnit)
	routes := make([]InboundRoute, 0)
	for _, e := range exts {
		if len(e.Inbound) == 0 {
			continue
		}
		byslug := make(map[string]extension.InboundEndpoint, len(e.Inbound))
		for _, endpoint := range e.Inbound {
			byslug[endpoint.Slug] = endpoint
			routes = append(routes, InboundRoute{
				Pattern:  inboundPrefix + string(e.Name) + "/" + endpoint.Slug,
				Unit:     e.Name,
				Endpoint: endpoint,
			})
		}
		units[string(e.Name)] = inboundUnit{name: e.Name, version: string(e.Version), endpoints: byslug}
	}
	if len(routes) == 0 {
		return routes
	}
	handler := &inboundHandler{
		units:   units,
		resolve: resolve,
		deps:    deps,
		log:     log,
		now:     time.Now,
		perIP:   inboundLimiters(units, func(e extension.InboundEndpoint) extension.Rate { return e.Rate.PerIP }),
		perSlug: inboundLimiters(units, func(e extension.InboundEndpoint) extension.Rate { return e.Rate.PerEndpoint }),
	}
	// Exact patterns per declared endpoint, never a prefix. A prefix would
	// answer for every path beneath it, so an undeclared slug would reach this
	// code and be refused there — turning "does this endpoint exist" into a
	// question the handler answers, which is exactly the enumeration the opaque
	// 401 exists to prevent. ServeMux answering 404 for what was never declared
	// is both cheaper and less informative.
	//
	// TWO patterns each, because a declared slug is the same for every caller
	// and a connector usually needs one URL per member. The second carries a
	// trailing {ref} the unit minted and resolves for itself; ServeMux still
	// matches only this exact shape, so a third segment is a 404 from the mux
	// rather than something this code has to consider.
	for _, route := range routes {
		wrapped := httpserver.Correlate(httpserver.AccessLog(log, handler))
		mux.Handle(route.Pattern, wrapped)
		mux.Handle(route.Pattern+"/{ref}", wrapped)
	}
	return routes
}

// inboundLimiters builds one limiter per declared endpoint, keyed by
// unit/slug. Per endpoint rather than one shared limiter, because each
// declaration asked for its own allowance and a shared bucket would let a busy
// endpoint spend a quiet one's budget.
func inboundLimiters(units map[string]inboundUnit, pick func(extension.InboundEndpoint) extension.Rate) map[string]*ratelimit.Limiter {
	out := make(map[string]*ratelimit.Limiter)
	for unit, u := range units {
		for slug, endpoint := range u.endpoints {
			rate := pick(endpoint)
			out[unit+"/"+slug] = ratelimit.New(rate.Limit, rate.Window)
		}
	}
	return out
}

// inboundHandler serves every declared inbound edge.
type inboundHandler struct {
	units   map[string]inboundUnit
	resolve workspaceResolver
	deps    extensionRuntimeBinding
	log     *slog.Logger
	now     func() time.Time
	perIP   map[string]*ratelimit.Limiter
	perSlug map[string]*ratelimit.Limiter
}

func (h *inboundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	unit, slug, ref, ok := splitInboundPath(r.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// Resolved against the DECLARATION before anything is keyed on it. A
	// limiter key built from a raw path segment is a self-serve way off the
	// meter, because ratelimit leaves an over-long key unmetered and admitted.
	u, known := h.units[unit]
	if !known {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	endpoint, declared := u.endpoints[slug]
	if !declared {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	key := unit + "/" + slug

	// Both budgets, both spent, before any work: everything below costs the
	// installation a body read, a secret decrypt and an HMAC.
	admitted := true
	if limiter := h.perIP[key]; limiter != nil && !limiter.Allow(httpserver.ClientIP(r)) {
		admitted = false
	}
	if limiter := h.perSlug[key]; limiter != nil && !limiter.Allow(key) {
		admitted = false
	}
	if !admitted {
		h.log.WarnContext(r.Context(), "inbound: over the metered rate", "unit", unit, "slug", slug)
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	body, ok := readInboundBody(w, r, endpoint.MaxBody)
	if !ok {
		return
	}

	stamp, nonce, signature, ok := inboundHeaders(r)
	if !ok {
		inboundRefuse(w)
		return
	}
	if !withinSkew(h.now(), stamp, endpoint.Skew) {
		inboundRefuse(w)
		return
	}

	// The tenant, from the installation itself. The slug deliberately does not
	// carry it: a slug that named the workspace would let an anonymous caller
	// choose which tenant its own request ran in.
	//
	// InstallationWorkspace CACHES a successful lookup, so a second workspace
	// appearing later keeps serving the cached one. That is a property of the
	// resolver and not of this edge, stated here because a reader tracing an
	// anonymous request's tenant will want to know it.
	//
	// Both failures — not bootstrapped, and more than one workspace — answer
	// ONE opaque 500. They are indistinguishable deployment faults to a caller,
	// and naming which one it is tells a stranger the shape of the installation.
	ws, err := h.resolve(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "inbound: resolving the installation's workspace",
			"unit", unit, "slug", slug, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	outcome, handleErr := h.invoke(r.Context(), ws, u, endpoint, extension.InboundRequest{
		Slug:      slug,
		Ref:       ref,
		Timestamp: stamp,
		Nonce:     nonce,
		Signature: signature,
		Body:      body,
	})
	h.answer(w, r, unit, slug, outcome, handleErr)
}

// invoke mints the request's Runtime and runs the unit's handler, releasing the
// Runtime whatever the handler does with it.
func (h *inboundHandler) invoke(
	ctx context.Context,
	ws ids.WorkspaceID,
	u inboundUnit,
	endpoint extension.InboundEndpoint,
	req extension.InboundRequest,
) (extension.InboundOutcome, error) {
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector,
		ID:   "connector:ext:" + string(u.name),
		// OnBehalfOf zero and Permissions empty: an anonymous edge carries no
		// authority. auth.Require has no connector branch, so a bare connector
		// passes exactly what its permissions allow, which is nothing.
	})
	rt := inboundRuntimeFor(ctx, string(u.name), u.version, inboundPrefix+string(u.name)+"/"+req.Slug, h.deps)
	defer rt.release()
	return endpoint.Handle(ctx, rt, req)
}

// answer maps the unit's outcome onto the status a remote sender sees.
func (h *inboundHandler) answer(w http.ResponseWriter, r *http.Request, unit, slug string, outcome extension.InboundOutcome, err error) {
	switch outcome {
	case extension.InboundAccepted:
		w.WriteHeader(http.StatusAccepted)
	case extension.InboundUnauthenticated:
		// Deliberately not logged with the reason at warn: the reasons are what
		// the identical 401 exists to hide, and a log line is where they would
		// reappear for anyone who can read one.
		inboundRefuse(w)
	case extension.InboundOverCapacity:
		h.log.WarnContext(r.Context(), "inbound: the unit is at capacity", "unit", unit, "slug", slug)
		w.WriteHeader(http.StatusTooManyRequests)
	case extension.InboundPoison:
		if err != nil {
			h.log.WarnContext(r.Context(), "inbound: poison payload", "unit", unit, "slug", slug, "err", err)
		}
		w.WriteHeader(http.StatusAccepted)
	case extension.InboundTransient:
		h.log.ErrorContext(r.Context(), "inbound: transient fault", "unit", unit, "slug", slug, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
	default:
		// A handler returning none of the declared outcomes is a bug in the
		// unit, not a fact about this request.
		h.log.ErrorContext(r.Context(), "inbound: handler returned an unrecognized outcome",
			"unit", unit, "slug", slug, "outcome", int(outcome))
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// inboundRefuse writes the one opaque refusal. Every reason a request may fail
// to authenticate answers exactly this: an unknown endpoint, a disabled one, a
// stale timestamp, a replayed nonce, a wrong signature. A response that told
// them apart would enumerate the installation's endpoints for whoever asked.
func inboundRefuse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
}

// readInboundBody reads at most limit bytes, answering the caller directly on
// failure and reporting whether the body is usable.
func readInboundBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// Over-cap or truncated. 413 rather than the opaque 401: the size of a
		// request is something the sender already knows, so saying so reveals
		// nothing, and a sender told "too large" fixes it where one told
		// "unauthorized" retries the same body forever.
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return nil, false
	}
	return body, true
}

// inboundHeaders reads the three signed headers. A missing or unparseable
// timestamp is not a request this edge can judge, so it is refused rather than
// admitted with a zero time.
func inboundHeaders(r *http.Request) (stamp time.Time, nonce, signature string, ok bool) {
	secs, err := strconv.ParseInt(r.Header.Get(extension.InboundHeaderTimestamp), 10, 64)
	if err != nil {
		return time.Time{}, "", "", false
	}
	nonce = r.Header.Get(extension.InboundHeaderNonce)
	signature = r.Header.Get(extension.InboundHeaderSignature)
	if nonce == "" || signature == "" {
		return time.Time{}, "", "", false
	}
	return time.Unix(secs, 0), nonce, signature, true
}

// splitInboundPath takes the unit, the slug and the optional trailing ref out of
// a mounted path.
//
// The unit and slug are read only to look them up against what was DECLARED —
// neither is ever used as a key before that lookup succeeds. The ref is never
// looked up here at all: it is handed to the unit, which resolves it against its
// own table, and it is deliberately kept out of every limiter key for the reason
// ratelimit states — an over-long key is unmetered and admitted, so a
// caller-chosen value must not be one.
func splitInboundPath(path string) (unit, slug, ref string, ok bool) {
	rest, found := strings.CutPrefix(path, inboundPrefix)
	if !found {
		return "", "", "", false
	}
	unit, rest, found = strings.Cut(rest, "/")
	if !found || unit == "" {
		return "", "", "", false
	}
	slug, ref, hasRef := strings.Cut(rest, "/")
	if slug == "" {
		return "", "", "", false
	}
	if !hasRef {
		return unit, slug, "", true
	}
	// Bounded and grammatical before it travels any further. A ref outside the
	// published rule is a 404 rather than an empty one passed on: a unit that
	// received "" for a value the caller did spell would resolve the wrong row,
	// or none, and answer the same opaque 401 either way.
	if !extension.ValidInboundRef(ref) {
		return "", "", "", false
	}
	return unit, slug, ref, true
}
