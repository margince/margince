// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// Handlers is this module's slice of the contract surface; compose embeds it.
type Handlers struct {
	store *Store
}

// NewHandlers binds the transport to a store on the given pool.
//
// The store it builds carries the module's refusing seam defaults, so a
// composition that forgets WithCompanyEdges fails closed rather than creating
// projects no company page can find. HandlersOver is how compose hands in the
// store it has already wired.
func NewHandlers(db *database.DB) Handlers {
	return Handlers{store: NewStore(db)}
}

// HandlersOver binds the transport to a store the caller has already built, so
// the HTTP surface and every other reader share ONE store rather than two
// spellings of one — the second of which would be the one nobody remembered to
// wire.
func HandlersOver(store *Store) Handlers {
	return Handlers{store: store}
}

// WithFieldCatalog wires the workspace custom-field catalog into the
// transport's store (see Store.WithFieldCatalog); compose injects
// modules/customfields' Service here.
func (h Handlers) WithFieldCatalog(catalog fieldcatalog.Reader) Handlers {
	h.store = h.store.WithFieldCatalog(catalog)
	return h
}

// Store exposes the handlers' store to the composition layer, which injects
// this module's reads into the modules that may not import it.
func (h Handlers) Store() *Store {
	return h.store
}

// predicateAlways is a WHERE fragment that admits every row.
//
// It is what an absent clause becomes on the way into a statement: auth answers
// "" for a caller bounded by nothing and a nil filter has nothing to say, and an
// empty string interpolated into a WHERE is a syntax error rather than "no
// restriction".
const predicateAlways = "true"

// edgeBound resolves the edge's read admission and returns the clause bounding
// WHICH edges, admitting every edge for a caller bounded by nothing. Knowing a
// project exists does not license learning who sits on it, and the endpoint
// grants do not cover the pair.
func edgeBound(ctx context.Context, alias string, arg func(any) int) (string, error) {
	clause, err := auth.EdgeReadScope(ctx, alias, arg)
	if err != nil {
		return "", err
	}
	if clause == "" {
		return predicateAlways, nil
	}
	return clause, nil
}

// uuidPtr carries an optional typed id onto the wire.
func uuidPtr(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	converted := openapi_types.UUID(*id)
	return &converted
}

// RequiredFieldError names a field the body left empty that the record cannot
// be written without. It carries the field so every surface reports the same
// one.
type RequiredFieldError struct{ Field string }

func (e *RequiredFieldError) Error() string { return e.Field + " is required" }

// FieldFault names the missing required field, on every surface.
func (e *RequiredFieldError) FieldFault() (field, code, message string) {
	return e.Field, "required", e.Error()
}

// pathID asserts a contract path id as entity K's id — the widening point
// between the wire and the typed store surface (the route already names the
// entity, so the assertion lives here, not in the store).
func pathID[K ids.EntityKind](id crmcontracts.Id) ids.ID[K] {
	return ids.From[K](ids.UUID(id))
}

// requireBodyID refuses a required non-pointer id the body simply omitted.
func requireBodyID(field string, id openapi_types.UUID) error {
	return httperr.RequireBodyID(field, ids.UUID(id))
}

// idArg converts an optional wire uuid into this module's typed id.
func idArg[K ids.EntityKind](u *openapi_types.UUID) *ids.ID[K] {
	if u == nil {
		return nil
	}
	v := ids.From[K](ids.UUID(*u))
	return &v
}

func pageInfo(p storekit.Page) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: p.HasMore}
	if p.NextCursor != "" {
		info.NextCursor = &p.NextCursor
	}
	return info
}

// writeStoreErr maps a store error onto the wire the same way for every handler
// in this package: the defence-in-depth net below, then the sentinel registry.
func writeStoreErr(w http.ResponseWriter, r *http.Request, err error) {
	// A CHECK constraint is a business rule, so a breach that slipped past the
	// per-path validations still answers a typed 422 — never an opaque 500. The
	// constraint's NAME stays out of the body: it is schema, and the one a
	// caller can reach here is a runtime `cf_*_check` behind a picklist custom
	// field, whose name tells them our column and nothing they can act on. The
	// operator's log gets the name instead.
	if constraint, ok := storekit.CheckViolation(err); ok {
		slog.WarnContext(r.Context(), "a schema rule with no message of its own refused a write",
			"method", r.Method, "path", capabilitypath.Redact(r.URL.Path), "constraint", constraint)
		httperr.Write(w, r, httperr.Validation("body", "value_not_allowed",
			"a value violates a rule on this record; check the picklist options"))
		return
	}
	httperr.Write(w, r, err)
}
