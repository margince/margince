// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UpdateDSRInput is a patch to one request; a nil field leaves its column as it is.
type UpdateDSRInput struct {
	Status     *string
	AssigneeID *ids.UserID
	Resolution *string
	// ClearAssignee and ClearResolution carry an explicit null, which a nil
	// pointer cannot tell apart from a field the caller left out.
	ClearAssignee   bool
	ClearResolution bool
	// DisputeOpen opens or closes a complaint about the request.
	DisputeOpen *bool
	// Proof is what the closing patch states for the request's proof.
	Proof ProofStatement
}

// hasResolution reports whether an update carries (or the row already
// stores) an actual answer. A nil-only check would accept "" or
// whitespace-only text, which the "closing needs an answer" rule refuses.
func hasResolution(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

// validateDSRUpdate holds every UpdateDSR precondition: the transition map,
// and a nonblank answer on any request it leaves closed. FulfilErasure runs it
// before the erase, so a request that cannot close is never erased.
func validateDSRUpdate(current dsrRow, in UpdateDSRInput) *ValidationError {
	next := current.Status
	if in.Status != nil && *in.Status != current.Status {
		if !dsrTransitions[current.Status][*in.Status] {
			return illegalTransition(current.Status, *in.Status)
		}
		next = *in.Status
	}
	if !closedDSRStatus(next) {
		return nil
	}
	answer := current.Resolution
	switch {
	case in.ClearResolution:
		answer = nil
	case in.Resolution != nil:
		if closedDSRStatus(current.Status) && !sameAnswer(current.Resolution, in.Resolution) {
			return &ValidationError{Field: fieldResolution, Reason: "a closed request keeps its answer"}
		}
		answer = in.Resolution
	case next == current.Status:
		// A patch that neither closes nor writes the answer leaves the stored one as it is.
		return nil
	}
	if hasResolution(answer) {
		return nil
	}
	if next != current.Status {
		return &ValidationError{Field: fieldResolution, Reason: "closing a request needs its answer"}
	}
	return &ValidationError{Field: fieldResolution, Reason: "a closed request keeps its answer"}
}

// resolutionChanged is what the audit row claims about the answer. A write that
// leaves the stored text as it was, such as resending it, has not touched it.
func resolutionChanged(before, after *string) bool {
	if before == nil || after == nil {
		return before != after
	}
	return *before != *after
}

// sameAnswer lets a closed request take its own text again, so a client that
// resends the whole record is not refused for changing nothing.
func sameAnswer(stored, sent *string) bool {
	return stored != nil && sent != nil && *stored == *sent
}

// dsrHeldSets are the assignments both update writers share, beside the
// status. An explicit clear empties the assignee; an absent value keeps it.
func dsrHeldSets(in UpdateDSRInput, arg func(any) int) string {
	return storekit.SQLf("assignee_id = CASE WHEN $%d THEN NULL ELSE coalesce($%d, assignee_id) END, dispute_open = coalesce($%d, dispute_open)",
		arg(in.ClearAssignee), arg(in.AssigneeID), arg(in.DisputeOpen))
}

// UpdateDSR applies a patch under the request's row lock, refusing any transition
// validateDSRUpdate refuses and any hand-closed erasure.
func (s *Store) UpdateDSR(ctx context.Context, id ids.UUID, in UpdateDSRInput) (dsrRow, error) {
	if err := requireDSRAdmin(ctx, principal.ActionUpdate); err != nil {
		return dsrRow{}, err
	}
	var out dsrRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := lockDSR(ctx, tx, id)
		if err != nil {
			return err
		}
		if verr := validateDSRUpdate(current, in); verr != nil {
			return verr
		}
		if current.Kind == dsrKindErasure && closesAs(current, in, dsrStatusFulfilled) {
			return &ValidationError{Field: fieldStatus, Reason: "an erasure request is fulfilled by carrying out the erasure"}
		}
		if dsrUpdateChangesNothing(current, in) {
			out = current
			return nil
		}
		source, err := s.readProofSource(ctx, tx, current, in)
		if err != nil {
			return err
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		held := dsrHeldSets(in, arg)
		sql := storekit.SQLf(`
			UPDATE data_subject_request SET
			  status = coalesce($%d, status),
			  %s,
			  resolution = CASE WHEN $%d THEN NULL ELSE coalesce($%d, resolution) END
			WHERE id = $%d
			RETURNING `+dsrColumns, arg(in.Status), held, arg(in.ClearResolution), arg(in.Resolution), arg(id))
		if out, err = scanDSR(tx.QueryRow(ctx, sql, args...)); err != nil {
			return err
		}
		if err := auditDSRUpdate(ctx, tx, current, out); err != nil {
			return err
		}
		return writeRequestProof(ctx, tx, source, out, nil)
	})
	return out, err
}

// auditDSRUpdate is the update audit both writers share. The images carry the
// assignee, so a hand-back says who let the request go.
func auditDSRUpdate(ctx context.Context, tx pgx.Tx, current, out dsrRow) error {
	_, err := storekit.Audit(ctx, tx, "update", "data_subject_request", out.ID, map[string]any{
		fieldStatus: current.Status, fieldAssigneeID: current.AssigneeID, fieldDispute: current.DisputeOpen,
	}, map[string]any{
		fieldStatus: out.Status, fieldAssigneeID: out.AssigneeID, fieldResolution: resolutionChanged(current.Resolution, out.Resolution),
		fieldDispute: out.DisputeOpen,
	})
	return err
}
