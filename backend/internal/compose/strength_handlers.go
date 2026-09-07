// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The HTTP transport for GET /people/{id}/strength and
// GET /organizations/{id}/strength (§4 relationship strength): binds the
// path id to the typed store call and maps its result onto the
// generated wire shape. The store methods gate themselves (auth.Require
// + auth.EnsureVisible), so this file is pure edge + shape translation —
// no re-gating here.
//
// One wrinkle: PersonStrength/OrganizationStrength compute their inputs
// with aggregate SQL (max/count), which always answers one row — even
// for an id that was never there — so EnsureVisible's existence check
// is the only thing standing between an unbounded (admin) caller and a
// row that doesn't exist, and EnsureVisible skips that probe entirely
// for unbounded callers (the same gap orgrollupread.go documents and
// works around). GetPerson/GetOrganization's own SELECT has no such
// gap — a missing row is a missing row in its result set — so this file
// calls them first, purely to inherit their existence-hiding 404; their
// own auth.Require/EnsureVisible calls are redundant with the strength
// call's but idempotent, never a second, different gate.
import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// strengthHandlers shadows the generated GetPersonStrength /
// GetOrganizationStrength stubs over people's §4 computation.
type strengthHandlers struct {
	people *people.Store
	// pool names the activities behind the score. The computation lives in
	// `people`, which may not import `activities`, so the receipts are named
	// here — one batched read per response, under the caller's own gates.
	pool *pgxpool.Pool
	// now is the read's clock (newServer defaults it to time.Now), matching
	// orgRollupHandlers' shape.
	now func() time.Time
}

// named answers the score with its receipts named, or with the number alone.
//
// A failure to name them fails the response rather than serving the bare
// count: a page that silently degraded to an unopenable number is the defect
// this field removes, wearing a 200.
func (h strengthHandlers) named(
	ctx context.Context, wire crmcontracts.RelationshipStrength,
) (crmcontracts.RelationshipStrength, error) {
	if wire.ContributingActivityIds == nil {
		return wire, nil
	}
	ids := activityIDsOf(*wire.ContributingActivityIds)
	named, err := namedActivitiesFor(ctx, h.pool, ids)
	if err != nil {
		return crmcontracts.RelationshipStrength{}, err
	}
	wire.ContributingActivities = &named
	return wire, nil
}

// GetPersonStrength implements GET /people/{id}/strength.
func (h strengthHandlers) GetPersonStrength(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	personID := ids.From[ids.PersonKind](ids.UUID(id))
	if _, err := h.people.GetPerson(r.Context(), personID, storekit.IncludeArchived); err != nil {
		httperr.Write(w, r, err)
		return
	}
	now := h.now()
	rs, err := h.people.PersonStrength(r.Context(), personID, now)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	wire, err := h.named(r.Context(), people.StrengthToWire(rs, now))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wire)
}

// GetOrganizationStrength implements GET /organizations/{id}/strength.
func (h strengthHandlers) GetOrganizationStrength(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	orgID := ids.From[ids.OrganizationKind](ids.UUID(id))
	if _, err := h.people.GetOrganization(r.Context(), orgID, storekit.IncludeArchived); err != nil {
		httperr.Write(w, r, err)
		return
	}
	now := h.now()
	account, err := h.people.OrganizationStrength(r.Context(), orgID, now)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// This route answers the shared RelationshipStrength shape; the
	// account-only facts (contributor, contact count) ride the 360's
	// OrganizationStrength schema instead of widening this one.
	wire, err := h.named(r.Context(), people.StrengthToWire(account.RelationshipStrength, now))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wire)
}
