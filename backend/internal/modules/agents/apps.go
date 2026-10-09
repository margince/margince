// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The App extension on the wire: how a tool advertises its view, how a view
// declares what it may reach, and the ONE place either is rendered.
//
// This is SEP-1865 (extension revision 2026-01-26), which is not part of the
// core protocol revision the transport negotiates — a host opts into it per
// request, and one that does not is served the surface exactly as it was
// before any view existed.
//
// WHY THE RENDERING IS HERE AND NOT AT EACH SURFACE. `_meta.ui` reaches a
// client from two places — a tool in tools/list, a document in
// resources/list and resources/read — and the two halves are one promise: the
// tool names a view, the view states its limits. Rendered where they are
// served, they would be two spellings of one contract, and the failure that
// matters is precisely the one where they disagree.
//
// NOTHING HERE CARRIES AUTHORITY. A view is a second renderer for an answer a
// tool already gives in text. It holds no credential, spends no scope, and
// cannot reach a record its tool would not have answered — which is why none
// of this appears anywhere near the admission gate.

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// extensionUI is the App extension's identifier, used in the three places that
// must agree: what a client declares in its per-request capabilities, what
// server/discover advertises, and which requests are served `_meta.ui`. One
// spelling, for the reason extensionTasks gives.
const extensionUI = "io.modelcontextprotocol/ui"

// metaUIKey is the member both halves hang under, inside the `_meta` object the
// protocol reserves for extensions.
const metaUIKey = "ui"

// declaresUI reports whether this request may be offered a view.
//
// It is STRICTER than the presence check every other extension takes, because
// this extension's declaration carries a payload rather than being a bare
// acknowledgement: a client declares the content types it can render, and
// `mimeTypes` is required of it. A host that renders some other profile is not a
// host that can render these documents, so presence alone would offer a view to a
// client whose own declaration says it cannot show it.
//
// Fails closed for every reason declaresExtension gives, and additionally for a
// declaration with no `mimeTypes`, one that is not an array of strings, or one
// that does not name this surface's profile. What a false costs is the plain
// unrendered answer every client already handles.
func declaresUI(capabilities json.RawMessage) bool {
	declared, present := declaredExtension(capabilities, extensionUI)
	if !present {
		return false
	}
	// Decoded through a MAP and matched exactly, for the same reason
	// declaredExtension does it: a struct member would match `"MimeTypes"` or
	// `"MIMETYPES"` case-insensitively, and this check exists precisely to hold a
	// client to what it declared.
	var members map[string]json.RawMessage
	if err := json.Unmarshal(declared, &members); err != nil {
		return false
	}
	var offeredTypes []string
	if err := json.Unmarshal(members["mimeTypes"], &offeredTypes); err != nil {
		return false
	}
	return slices.Contains(offeredTypes, mcp.AppMIMEType)
}

// assertViewDeclaration holds a tool's own view declaration to the three things
// that can be judged from ONE spec, at the one door every tool comes through.
//
// What it deliberately does NOT check is whether the named document exists. That
// is a cross-reference between two things injected independently — the registry
// and a resource provider — and neither knows about the other here. The composed
// surface answers it instead (compose's App sweep), where both halves are known
// and the failure is a build that does not go green rather than a process that
// does not start.
func assertViewDeclaration(spec mcp.ToolSpec) error {
	if spec.UI == nil {
		return nil
	}
	// A view with no URI, or one outside the scheme, names nothing a host will
	// fetch. A host dispatches on `ui://` to tell a view from a document, so a
	// well-formed URI in any other scheme is not a near miss — it is a tool that
	// advertises a view no host will ever render, silently.
	if !strings.HasPrefix(spec.UI.ResourceURI, mcp.AppURIScheme) {
		return fmt.Errorf("%s declares the view %q, which is not a %s URI — a host tells a view from an ordinary document by that scheme, "+
			"so this tool would advertise a view nothing renders", spec.Name, spec.UI.ResourceURI, mcp.AppURIScheme)
	}
	modelReaches := len(spec.UI.Visibility) == 0
	for _, audience := range spec.UI.Visibility {
		switch audience {
		case mcp.VisibilityModel:
			modelReaches = true
		case mcp.VisibilityApp:
		default:
			// An audience no host recognises does not widen the list, it
			// narrows the tool: a host reads the entries it knows and this one
			// is not among them, so the reach the author meant is not the reach
			// the tool gets.
			return fmt.Errorf("%s declares the audience %q, which is not %q or %q — a host reads only the audiences it knows, "+
				"so an unrecognised one narrows this tool rather than widening it",
				spec.Name, audience, mcp.VisibilityModel, mcp.VisibilityApp)
		}
	}
	// A tool the model cannot select is a capability that exists only inside a
	// rendered view. A host MUST leave such a tool out of the agent's catalog,
	// so every client that does not render — every script, every CI check, every
	// host without the extension — loses the capability entirely. This surface
	// serves views as a second renderer for an answer a tool already gives, and
	// that promise is exactly this check.
	if !modelReaches {
		return fmt.Errorf("%s is declared visible to %q alone, which makes it a capability only a rendered view can reach — "+
			"a view is a second renderer for an answer this tool already gives, never its only door",
			spec.Name, mcp.VisibilityApp)
	}
	return nil
}

// appsServed reports whether this server has any view to offer — a tool that
// declares one AND a document that is actually being served for it.
//
// BOTH HALVES, and the second one is the whole point. A tool's UI.ResourceURI is
// a constant baked at registration, so a registry-only answer says "yes" for a
// view whose document never arrived. The protocol makes it a MUST that a `ui://`
// URI a tool names exists on the server, and a host is entitled to prefetch one
// before the tool is ever called — so advertising the extension on the strength
// of a declaration alone sends every such host to a not-found.
//
// A deployment with no view provider wired answers false and serves the surface
// exactly as it did before any view existed, which is the same conditional
// wiring every other injected capability takes.
func (s *Dispatcher) appsServed() bool {
	return slices.ContainsFunc(s.registry.Specs(), s.viewIsHeld)
}

// viewIsHeld reports whether THIS tool's view is one the server is serving.
//
// It is the per-tool question `_meta.ui` turns on, and it is asked per tool
// rather than once per request because partial availability is a real state: one
// view missing must suppress ONE tool's entry, not both, and must never withdraw
// the tool itself.
func (s *Dispatcher) viewIsHeld(spec mcp.ToolSpec) bool {
	if spec.UI == nil || s.viewHeld == nil {
		return false
	}
	return s.viewHeld(spec.UI.ResourceURI)
}

// appsOffered reports whether THIS request is served the App extension's
// members: this server has views to declare, and nothing about the request says
// not to offer them.
//
// The SERVED half is unconditional. A server with no view that advertised one
// sends every prefetching host to a not-found, and no client declaration can
// conjure a document.
//
// The DECLARED half is asked of the modern era only, and that asymmetry is the
// whole of this function. A modern request negotiates per call, so a client that
// COULD have declared the extension and did not is a client saying no, and
// offering anyway would override an answer it gave. The handshake era gives no
// such answer: it has no `_meta` to carry one, and reading silence there as "no"
// withheld the feature from every client in that era — which, measured against
// Claude Desktop on 2026-08-11, is the era the hosts that actually render views
// still connect in. `_meta` is the member the protocol reserves for exactly this
// and instructs a receiver to ignore what it does not understand, so the cost of
// offering a view to a handshake client that cannot draw one is the bytes; the
// cost of withholding it is the feature.
func (s *Dispatcher) appsOffered(fr framing) bool {
	if !s.appsServed() {
		return false
	}
	if fr.modern {
		return fr.apps
	}
	return true
}

// toolUIWire is one tool's `_meta.ui` as a client reads it.
type toolUIWire struct {
	//nolint:tagliatelle // resourceUri is the extension's wire member, camelCase by the specification
	ResourceURI string   `json:"resourceUri,omitempty"`
	Visibility  []string `json:"visibility"`
}

// toolUIMeta renders a tool's view declaration, and answers nil for a tool
// that has none — which is most of them, and which is why the absence is a
// return value rather than an empty object. A `_meta` member on every tool
// would be catalog bytes spent, on every client, for the tools that have no
// view.
func toolUIMeta(spec mcp.ToolSpec) *toolUIWire {
	if spec.UI == nil {
		return nil
	}
	return &toolUIWire{
		ResourceURI: spec.UI.ResourceURI,
		Visibility:  visibilityOrBoth(spec.UI.Visibility),
	}
}

// toolMeta is a tool's `_meta.ui` for a request that is offered views: its own
// declaration when its view is held, nothing for a tool a view acts through
// (the default audience, both, is what it needs), and model-only for every
// other tool.
func (s *Dispatcher) toolMeta(spec mcp.ToolSpec, actionTools map[string]struct{}) *toolUIWire {
	if s.viewIsHeld(spec) {
		if ui := toolUIMeta(spec); ui != nil {
			return ui
		}
	}
	if _, acts := actionTools[spec.Name]; acts {
		return nil
	}
	return viewOnlyModelMeta()
}

// viewOnlyModelMeta is what a tool no view acts through is declared as: the
// model's alone. The extension's default audience is both, so without this a
// rendered view could ask its host to run ANY tool on the surface; with it, the
// only tools a view can reach are the ones apps.ActionTools names.
func viewOnlyModelMeta() *toolUIWire {
	return &toolUIWire{Visibility: []string{mcp.VisibilityModel}}
}

// visibilityOrBoth resolves an undeclared audience list to BOTH audiences.
//
// The default cannot be the zero value passed through, because an empty list is
// the protocol's own spelling for "no audience": a host MUST leave a tool out
// of the model's catalog when `visibility` excludes "model", so serving `[]`
// would withdraw from the model a tool that was model-callable before it grew a
// view. Saying nothing about audience has to mean the audience did not change.
func visibilityOrBoth(declared []string) []string {
	if len(declared) > 0 {
		return declared
	}
	return []string{mcp.VisibilityModel, mcp.VisibilityApp}
}

// resourceUIWire is one view's `_meta.ui` as a host reads it before it builds
// the sandbox.
type resourceUIWire struct {
	// No `permissions` member: the extension reads a permission member's
	// PRESENCE as the request, so a view asking for none omits it.
	CSP    resourceCSPWire `json:"csp"`
	Domain string          `json:"domain,omitempty"`
	//nolint:tagliatelle // prefersBorder is the extension's wire member, camelCase by the specification
	PrefersBorder bool `json:"prefersBorder,omitempty"`
}

// resourceCSPWire carries the four allowlists. NONE of them is `omitempty`, and
// every one is normalized to an empty array rather than left nil — see
// closedList.
type resourceCSPWire struct {
	//nolint:tagliatelle // the four domain members are the extension's own, camelCase by the specification
	ConnectDomains []string `json:"connectDomains"`
	//nolint:tagliatelle // as above
	ResourceDomains []string `json:"resourceDomains"`
	//nolint:tagliatelle // as above
	FrameDomains []string `json:"frameDomains"`
	//nolint:tagliatelle // as above
	BaseURIDomains []string `json:"baseUriDomains"`
}

// resourceUIMeta renders a view's own declaration, and answers nil for an
// ordinary document — a sandbox policy on something no host will sandbox is a
// claim about nothing.
func resourceUIMeta(resource mcp.Resource) *resourceUIWire {
	if resource.UI == nil {
		return nil
	}
	return &resourceUIWire{
		CSP: resourceCSPWire{
			// A host builds its content-security policy from these and MUST NOT
			// admit an origin they do not name, so an empty list is an
			// instruction to deny everything. `null` would read as unspecified.
			ConnectDomains:  closedList(resource.UI.CSP.ConnectDomains),
			ResourceDomains: closedList(resource.UI.CSP.ResourceDomains),
			FrameDomains:    closedList(resource.UI.CSP.FrameDomains),
			BaseURIDomains:  closedList(resource.UI.CSP.BaseURIDomains),
		},
		Domain:        resource.UI.Domain,
		PrefersBorder: resource.UI.PrefersBorder,
	}
}

// closedList normalizes a list so the wire carries `[]` and never `null`.
//
// The two look alike in Go and say opposite things to a reader: `[]` is a
// closed list holding nothing, `null` is a list that was not stated — an
// omission, and an omission is where a permissive default lives. Both callers
// depend on that distinction for their own reason, and each says so at the call.
func closedList(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}
