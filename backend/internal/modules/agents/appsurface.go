// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// AppSurface is the fixed tool list an installed app calls this server with.
// The app reads tools/list once, with one caller's token, and shows that list
// to all of its users. So the list cannot depend on who asked.
type AppSurface struct {
	// Tools are the only names the app may list and call.
	Tools []string
	// Seatless answers a caller the app vouches for who is no member. Such a
	// caller may read the list and nothing else.
	Seatless EdgeRefusal
}

type appSurfaceKey struct{}

// WithAppSurface binds the calling app's surface to the request. The edge that
// verified the app's token sets it.
func WithAppSurface(ctx context.Context, surface AppSurface) context.Context {
	return context.WithValue(ctx, appSurfaceKey{}, surface)
}

func appSurfaceOf(ctx context.Context) (AppSurface, bool) {
	surface, ok := ctx.Value(appSurfaceKey{}).(AppSurface)
	return surface, ok
}

// seatlessMethods are what a seatless app caller is served: the handshake and
// the lists an app reads when it refreshes. An app is served no resources and
// no prompts, so those lists are empty. Everything else needs a member.
var seatlessMethods = []string{
	methodInitialize, methodPing, methodDiscover, methodToolsList,
	methodResourcesList, methodResourceTemplatesList, methodPromptsList,
}

// seatlessRefusal answers a method a seatless app caller may not use, and nil
// for any other request.
//
//nolint:ireturn // EdgeRefusal is the edge's refusal seam; nil means admit.
func seatlessRefusal(ctx context.Context, method string) EdgeRefusal {
	surface, ok := appSurfaceOf(ctx)
	if !ok || surface.Seatless == nil || slices.Contains(seatlessMethods, method) {
		return nil
	}
	return surface.Seatless
}

// offeredTo is the list this caller is shown: an app's fixed list, or what the
// caller's passport and grants are offered.
func (s *Dispatcher) offeredTo(ctx context.Context) []mcp.ToolSpec {
	if surface, ok := appSurfaceOf(ctx); ok {
		return s.registry.Fixed(surface.Tools)
	}
	return s.registry.Offered(ctx)
}

// offeredOne is the one tool name as offeredTo would list it to this caller,
// and false when that list leaves it out.
func (r *Registry) offeredOne(ctx context.Context, name string) (mcp.ToolSpec, bool) {
	spec, ok := r.Spec(name)
	if !ok {
		return spec, false
	}
	if surface, isApp := appSurfaceOf(ctx); isApp {
		if !slices.Contains(surface.Tools, name) {
			return spec, false
		}
		return r.availableSpec(spec)
	}
	if spec, ok = r.availableSpec(spec); !ok {
		return spec, false
	}
	return r.offeredSpec(ctx, spec)
}

// outsideAppSurface reports a name the calling app may not call. ok is false
// when no app is calling.
func outsideAppSurface(ctx context.Context, name string) (outside, ok bool) {
	surface, ok := appSurfaceOf(ctx)
	if !ok {
		return false, false
	}
	return !slices.Contains(surface.Tools, name), true
}

// Fixed lists the named tools as this deployment serves them, in the order
// named. A name nothing registers is left out. Each call still meets the
// caller's own scopes and grants at Invoke.
func (r *Registry) Fixed(names []string) []mcp.ToolSpec {
	out := make([]mcp.ToolSpec, 0, len(names))
	for _, name := range names {
		spec, ok := r.Spec(name)
		if !ok {
			continue
		}
		if available, ok := r.availableSpec(spec); ok {
			out = append(out, available)
		}
	}
	return out
}

// IsAppCall reports a request an installed app made.
func IsAppCall(ctx context.Context) bool {
	_, ok := appSurfaceOf(ctx)
	return ok
}
