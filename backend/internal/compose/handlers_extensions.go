// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The composed-extension inventory (GET /v1/extensions): what this binary
// actually composed, per unit.
//
// It exists for the role editor. An extension's objects reach a role document as
// `ext_<unit>_<object>` and nothing else in the grant map says which product
// surface a name belongs to — an operator reading a flat list of thirty core
// objects and two ext_ ones has no way to know that granting one of them is what
// turns on the notes screen. This answers that grouping question, and answers it
// from the same values the boot reconciliation validated.
//
// It lives in compose rather than in a module because the composed set is not a
// module's fact: it is the composition root's, held in this package's own
// accessors (ComposedExtensions, ComposedVerbs, servedExtensionJobs). A module
// asked to serve this would need those handed to it, which is a dependency in
// the wrong direction for a value that is fixed at build time.

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// extensionsHandlers serves the inventory. What a unit is comes from the
// process-level accessors. The pool and the app store answer only which
// declared setup an admin has done (setupMissing).
type extensionsHandlers struct {
	pool *pgxpool.Pool
	// apps is nil where no key vault is wired, so no OAuth app can be stored.
	apps *capture.ConnectorAppStore
}

// ListExtensions (GET /v1/extensions). Gated on extension_access.read.
//
// The check is taken HERE rather than in a service because there is no service:
// nothing is read from the database, so there is no store to own the gate.
//
// A grant and not the literal admin role, so an installation can hand the
// inventory to an operator without handing them member administration with it —
// admin and ops hold the read by default. The bar is still an operator one even
// though the answer is identical for every caller and changes only on deploy: it
// enumerates the installation's internal surface (routes, jobs, unit versions).
//
// Read is the only verb it will ever carry. The instance's go.mod is the
// enablement, so there is no runtime toggle for a write to gate.
//
// Agents never reach it at all: the operation is `x-agent-access: human-only`,
// enforced by the generated policy table before this runs.
func (h extensionsHandlers) ListExtensions(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r.Context(), "extension_access", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	exts := ComposedExtensions()
	missing := make(map[string]*[]crmcontracts.ComposedExtensionSetupItem, len(exts))
	for _, e := range exts {
		items, known, err := h.setupMissing(r.Context(), e)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		if known {
			missing[string(e.Name)] = &items
		}
	}
	units := composedExtensionInventory(exts, ComposedVerbs(), servedExtensionJobs())
	for i := range units {
		units[i].SetupMissing = missing[units[i].Name]
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ExtensionDirectory{Extensions: units})
}

// setupMissing names what an admin still has to set for the unit. Each part
// needs the caller's own right: admin for a workspace secret, as for the
// unit's declared settings, and capture_settings.read for an OAuth app. A part
// the caller may not read is left out. If one is left out and nothing visible
// is missing, known is false, so the caller is never told the unit is ready.
func (h extensionsHandlers) setupMissing(ctx context.Context, e extension.Extension) (missing []crmcontracts.ComposedExtensionSetupItem, known bool, err error) {
	see := setupVisibility{
		secrets: auth.AdminGranted(ctx),
		apps:    auth.Require(ctx, "capture_settings", principal.ActionRead) == nil,
	}
	var keys []string
	for _, s := range e.Secrets {
		if see.secrets && s.Scope == extension.SecretScopeWorkspace {
			keys = append(keys, s.Key)
		}
	}
	stored := map[string]bool{}
	if len(keys) > 0 {
		if err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) (err error) {
			stored, err = extsecrets.WorkspaceKeysStored(ctx, tx, string(e.Name), keys)
			return err
		}); err != nil {
			return nil, false, err
		}
	}
	configured := map[string]bool{}
	for _, app := range e.OAuthApps {
		if h.apps == nil || !see.apps {
			break
		}
		status, err := h.apps.Read(ctx, capture.AppProvider(app.Provider))
		if err != nil {
			return nil, false, err
		}
		configured[app.Provider] = status.Configured
	}
	missing = unitSetupMissing(e, see, stored, configured)
	return missing, len(missing) > 0 || !see.hides(e), nil
}

// setupVisibility says which parts of a unit's setup the caller may read.
type setupVisibility struct{ secrets, apps bool }

// hides reports whether the unit declares a part of its setup the caller may
// not read.
func (see setupVisibility) hides(e extension.Extension) bool {
	workspaceSecret := slices.ContainsFunc(e.Secrets, func(s extension.SecretsRequest) bool { return s.Scope == extension.SecretScopeWorkspace })
	return (workspaceSecret && !see.secrets) || (len(e.OAuthApps) > 0 && !see.apps)
}

// unitSetupMissing lists, in declaration order, the visible parts still unset.
// That is each workspace secret with no value, then each OAuth app with no
// stored client. A user-scoped secret is each member's own and never counts.
func unitSetupMissing(e extension.Extension, see setupVisibility, stored, configured map[string]bool) []crmcontracts.ComposedExtensionSetupItem {
	out := []crmcontracts.ComposedExtensionSetupItem{}
	for _, s := range e.Secrets {
		if see.secrets && s.Scope == extension.SecretScopeWorkspace && !stored[s.Key] {
			out = append(out, crmcontracts.ComposedExtensionSetupItem{Kind: crmcontracts.ComposedExtensionSetupItemKindWorkspaceSecret, Name: s.Key})
		}
	}
	for _, app := range e.OAuthApps {
		if see.apps && !configured[app.Provider] {
			out = append(out, crmcontracts.ComposedExtensionSetupItem{Kind: crmcontracts.ComposedExtensionSetupItemKindOauthApp, Name: app.Title})
		}
	}
	return out
}

// composedExtensionInventory joins the three composed facts into one entry per
// unit. Split from the handler so the join is testable without a request, and
// so the handler stays the transport decision it should be.
//
// Every list is sorted and non-nil. Sorted because map iteration and slice order
// are not a UI's business and a re-render must not reshuffle; non-nil because
// `[]` on the wire says "this unit contributes none", while `null` says nothing
// at all — and "contributes no RBAC objects" is the common, correct answer for a
// unit that owns no records.
func composedExtensionInventory(exts []extension.Extension, verbs []extension.Verb, jobs []composedJob) []crmcontracts.ComposedExtension {
	out := make([]crmcontracts.ComposedExtension, 0, len(exts))
	for _, e := range exts {
		out = append(out, crmcontracts.ComposedExtension{
			Name:        string(e.Name),
			Version:     string(e.Version),
			Description: string(e.Description),
			RbacObjects: unitRbacObjects(e, verbs),
			Routes:      unitRoutes(e.Name, verbs),
			Jobs:        unitJobs(e.Name, jobs),
			Needs:       unitNeeds(e),
			MailHooks:   unitMailHooks(e.MailHooks),
		})
	}
	slices.SortFunc(out, func(a, b crmcontracts.ComposedExtension) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// unitNeeds lists what the unit declares it borrows from the core, sorted by
// capability. Nil for a unit that declares none, so the field is absent on the
// wire rather than an empty promise.
func unitNeeds(e extension.Extension) *[]crmcontracts.ComposedExtensionNeed {
	if len(e.Needs) == 0 {
		return nil
	}
	needs := make([]crmcontracts.ComposedExtensionNeed, 0, len(e.Needs))
	for _, need := range e.Needs {
		needs = append(needs, crmcontracts.ComposedExtensionNeed{Capability: string(need.Capability), Reason: need.Reason})
	}
	slices.SortFunc(needs, func(a, b crmcontracts.ComposedExtensionNeed) int { return strings.Compare(a.Capability, b.Capability) })
	return &needs
}

// unitMailHooks is what of each captured mail the unit's hooks read, or nil
// for a unit with no hook. The manifest records the same declaration.
func unitMailHooks(h extension.MailHooks) *crmcontracts.ComposedExtensionMailHooks {
	if h.Parties == nil && h.Deal == nil {
		return nil
	}
	reads := extension.MailViewReads()
	out := &crmcontracts.ComposedExtensionMailHooks{
		Parties: h.Parties != nil, Deal: h.Deal != nil,
		Reads:          make([]crmcontracts.ComposedExtensionMailHooksReads, len(reads)),
		BodyCharacters: extension.MailBodyCharacters,
		Reasons:        slices.Sorted(slices.Values(h.Reasons)),
	}
	for i, read := range reads {
		out.Reads[i] = crmcontracts.ComposedExtensionMailHooksReads(read)
	}
	return out
}

// unitRbacObjects collects the distinct objects this unit's operations gate on.
//
// Read off the VERBS rather than off the RBAC registry, deliberately. The
// registry (policy.RegisteredObjects) knows every registered name but not which
// unit declared it — the `ext_<unit>_` prefix looks like it would answer that
// and does not, because the namespace is not injective: a hyphenated unit name
// underscores, so `crm-demo`/`widget` and `crm`/`demo_widget` derive one name
// (see extrbac.go). Splitting the prefix back apart would attribute an object to
// the wrong unit; the verb carries its declaring unit as data and cannot.
//
// A record kind the unit declares adds its own object, read off the declaration
// for the same reason. Without it the role editor cannot place the object under
// its unit.
func unitRbacObjects(unit extension.Extension, verbs []extension.Verb) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range verbs {
		if v.Unit != unit.Name || v.RbacObject == "" || seen[v.RbacObject] {
			continue
		}
		seen[v.RbacObject] = true
		out = append(out, v.RbacObject)
	}
	for _, k := range unit.RecordKinds {
		object := extension.RecordKindObject(unit.Name, k.Kind)
		if !seen[object] {
			seen[object] = true
			out = append(out, object)
		}
	}
	slices.Sort(out)
	return out
}

// unitRoutes collects the unit's published operations as method+path pairs, one
// per operation rather than one per path. A bare path would under-report the
// surface: a path serving GET and DELETE is two capabilities to an operator
// auditing what a unit can do, and collapsing them hides the destructive one
// behind the harmless one.
//
// The CONTRACT spelling (`/ext/<unit>/…`), not the mounted path, because that is
// the spelling a reader can find in the published document — see Verb.Route.
//
// Sorted by path then method, so the pairs for one path stay adjacent — a list
// sorted by method first would scatter one endpoint's verbs across the whole
// unit.
func unitRoutes(unit extension.Name, verbs []extension.Verb) []crmcontracts.ComposedExtensionRoute {
	out := []crmcontracts.ComposedExtensionRoute{}
	for _, v := range verbs {
		if v.Unit != unit {
			continue
		}
		out = append(out, crmcontracts.ComposedExtensionRoute{Method: v.Method, Path: v.Route})
	}
	slices.SortFunc(out, func(a, b crmcontracts.ComposedExtensionRoute) int {
		if byPath := strings.Compare(a.Path, b.Path); byPath != 0 {
			return byPath
		}
		return strings.Compare(a.Method, b.Method)
	})
	return out
}

// unitJobs collects the jobs this boot RUNS for the unit — the served set, not
// the declared one. A job declared in the unit's jobs.yaml with no Go handler
// ticks nothing, and an operator reading this list is asking what runs; naming
// an inert kind here would promise a cadence nobody works.
func unitJobs(unit extension.Name, jobs []composedJob) []string {
	out := []string{}
	for _, j := range jobs {
		if j.decl.Unit == unit {
			out = append(out, j.decl.Job)
		}
	}
	slices.Sort(out)
	return out
}
