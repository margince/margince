// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The HTTP transport for the reads whose receipts compose names:
// GET /contacts/{id}/strength, GET /companies/{id}/strength, and, in
// leadscore_handlers.go, GET /leads/{id}/score.
//
// Each binds the path id to its typed store call and maps the result onto the
// generated wire shape. The store methods gate themselves through
// `auth.Require` and `auth.EnsureVisible`, so this file translates edge and
// shape and re-gates nothing.
//
// One wrinkle in the strength reads. `ContactStrength` and `CompanyStrength`
// compute their inputs with aggregate SQL, so they always answer one row,
// even for an id that was never there. `EnsureVisible`'s existence check is
// then the only thing between an unbounded caller and a row that does not
// exist. And it skips that probe for an unbounded caller. The same gap is
// recorded in companyrollupread.go.
//
// `GetContact` and `GetCompany` have no such gap, because a missing row is
// missing from their own result set. So each handler calls one of them first,
// to inherit its existence-hiding 404. Their own gate calls repeat the
// strength call's and are idempotent, never a second, different gate.

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// receiptHandlers is the edge for the contacts computations whose activity
// receipts are named in compose: the relationship strength and the lead
// score.
//
// A second such read joins this type rather than adding another seam. The
// pattern is one thing: a derived number computed in `contacts`, answered
// with the activities behind it named here.
type receiptHandlers struct {
	contacts *contacts.Store
	// pool names the activities behind the score. The computation lives in
	// `contacts`, which may not import `activities`, so the receipts are named
	// here: one batched read per response, under the caller's own gates.
	pool *pgxpool.Pool
	// now is the read's clock (newServer defaults it to time.Now), matching
	// companyRollupHandlers' shape.
	now func() time.Time
}

// namedStrength answers the score with its receipts named.
//
// A failure to name them fails the response rather than serving the bare
// count. A page that degraded to an unopenable number, wearing a 200, is the
// defect this field removes.
func (h receiptHandlers) namedStrength(
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

// GetContactStrength implements GET /contacts/{id}/strength.
func (h receiptHandlers) GetContactStrength(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	contactID := ids.From[ids.ContactKind](ids.UUID(id))
	if _, err := h.contacts.GetContact(r.Context(), contactID, storekit.IncludeArchived); err != nil {
		httperr.Write(w, r, err)
		return
	}
	now := h.now()
	rs, err := h.contacts.ContactStrength(r.Context(), contactID, now)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	wire, err := h.namedStrength(r.Context(), contacts.StrengthToWire(rs, now))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wire)
}

// GetCompanyStrength implements GET /companies/{id}/strength.
func (h receiptHandlers) GetCompanyStrength(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	companyID := ids.From[ids.CompanyKind](ids.UUID(id))
	if _, err := h.contacts.GetCompany(r.Context(), companyID, storekit.IncludeArchived); err != nil {
		httperr.Write(w, r, err)
		return
	}
	now := h.now()
	account, err := h.contacts.CompanyStrength(r.Context(), companyID, now)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// This route answers the shared RelationshipStrength shape; the
	// account-only facts (contributor, contact count) ride the 360's
	// CompanyStrength schema instead of widening this one.
	wire, err := h.namedStrength(r.Context(), contacts.StrengthToWire(account.RelationshipStrength, now))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wire)
}
