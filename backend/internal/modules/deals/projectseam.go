// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal needs from the project it names, as ports the composition root
// fills (ADR-0054 §3: a module never imports a sibling).
//
// Both run INSIDE the caller's transaction, which is what makes them answers
// rather than guesses: the attach check re-reads under the same lock the write
// takes, and the delivery advance rides the very transaction that recorded the
// win, so a won deal and a project that moved on it commit together or not at
// all.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// errSeamUnwired names the seam and what to construct instead, so an operator
// reading the log is told the fix rather than only the symptom.
func errSeamUnwired(seam string) error {
	return errors.New("deals: the " + seam + " seam was not injected; " +
		"construct this store with installseam.Deals(), which binds modules/projects to it")
}

// EnsureProjectAttachable refuses a project this caller may not write to, or
// one that is archived.
//
// The project pointer is held to a higher bar than the rest of a deal's fields:
// winning this deal advances the project's phase without re-checking the
// caller's authority over the project, so the authority is taken HERE, when the
// pointer is set.
type EnsureProjectAttachable func(ctx context.Context, tx pgx.Tx, projectID ids.UUID) error

// StartDeliveryForWonDeal moves the won deal's project into delivery, if it
// names one and its phase is behind delivery. A deal that names no project is a
// no-op, not an error.
type StartDeliveryForWonDeal func(ctx context.Context, tx pgx.Tx, dealID ids.DealID, by string) error

// refusingEnsureProjectAttachable is what an un-injected attach check becomes:
// it refuses every project rather than admitting every one. A seam that failed
// OPEN here would silently restore the hole it exists to close.
func refusingEnsureProjectAttachable() EnsureProjectAttachable {
	return func(context.Context, pgx.Tx, ids.UUID) error {
		return errSeamUnwired("EnsureProjectAttachable")
	}
}

// refusingStartDelivery is what an un-injected delivery advance becomes. It
// refuses rather than silently skipping: a win that quietly failed to move the
// project leaves the two records disagreeing with nothing saying why.
func refusingStartDelivery() StartDeliveryForWonDeal {
	return func(context.Context, pgx.Tx, ids.DealID, string) error {
		return errSeamUnwired("StartDeliveryForWonDeal")
	}
}

// dealProjectSameOrgConstraint is the constraint trigger that enforces "a deal
// and its project name the same company" — a rule spanning two rows, so it
// cannot be a CHECK, and its name is what the deal write paths match on to
// answer 422 rather than 500.
const dealProjectSameOrgConstraint = "deal_project_same_org"

// DealProjectOrgMismatchError maps to 422: a deal and the project it belongs to
// must name the same company. Raised by the deal_project_same_org constraint
// trigger, which is the only place the cross-row rule can be enforced.
//
// It lives with the deal rather than with the project because the write that
// trips it is a deal write, and the field it faults is the deal's.
type DealProjectOrgMismatchError struct{}

func (e *DealProjectOrgMismatchError) Error() string {
	return "a deal and its project must belong to the same company"
}

// FieldFault refuses linking a deal to a project under a different organization.
func (e *DealProjectOrgMismatchError) FieldFault() (field, code, message string) {
	return "project_id", "project_organization_mismatch", e.Error()
}
