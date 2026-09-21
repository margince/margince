// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The modify-then-approve arm: a human changed the staged payload before
// releasing it.
//
// Split from decide.go because it is one concept with its own rule — the
// edited payload REPLACES the staged one under a freshly computed diff_hash,
// so what applies is what the approver last saw and not what was proposed.
// Every other decision path leaves the payload untouched.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// applyEditedPayload is the modify-then-approve write (ADR-0036 §4): the
// human's edited payload replaces the staged change under a freshly
// computed diff_hash, and both sides of the human delta go on the record
// — what the agent proposed, and what the human actually released. The
// decided event carries the human's version, so a suspended agent run
// resumes with THIS call; the original hash no longer opens anything.
func applyEditedPayload(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, edited json.RawMessage, a row, auditEvidence map[string]any, decidedPayload *crmcontracts.PublicEventApprovalDecided) error {
	canonical, editedHash, hashErr := diffhash.Canonical(edited)
	if hashErr != nil {
		return &InvalidEditError{Cause: hashErr}
	}
	// The edit may correct the action, never re-aim it: the row-scope probe
	// and the version pin above were both evaluated against the records the
	// STAGED payload named, and the effect resolves what it writes from the
	// payload rather than from the approval's target. See editscope.go.
	if err := assertSameEntityRefs(a.ProposedChange, canonical); err != nil {
		return err
	}
	// The same rule for the half entityRefs cannot see: a record named inside the
	// request path rather than as a field of its own.
	if err := assertSameCallIdentity(a.ProposedChange, canonical); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE approval SET proposed_change = $2, diff_hash = $3 WHERE id = $1`,
		id, canonical, editedHash); err != nil {
		return err
	}
	auditEvidence["edited"] = true
	auditEvidence["original_change"] = json.RawMessage(a.ProposedChange)
	auditEvidence["original_diff_hash"] = a.DiffHash
	auditEvidence["edited_change"] = json.RawMessage(canonical)
	auditEvidence["edited_diff_hash"] = editedHash

	// edited_change stays an OPEN object on the wire (A9): the staged
	// kind's proposed_change shape varies by kind, so the payload carries
	// it as a raw map rather than a narrowly typed struct that would drop
	// a future kind's fields.
	var editedChange map[string]any
	if err := json.Unmarshal(canonical, &editedChange); err != nil {
		return fmt.Errorf("approvals: canonicalized edited change did not decode as a JSON object: %w", err)
	}
	if editedChange == nil {
		// A literal JSON `null` decodes without error but leaves the map nil,
		// which would emit edited_change: null (violating the public contract)
		// and could resume a parked run with null args — reject it as an
		// invalid edit (422) rather than a JSON object.
		return &InvalidEditError{Cause: errors.New("payload is not a JSON object")}
	}
	wasEdited := true
	decidedPayload.Edited = &wasEdited
	decidedPayload.DiffHash = &editedHash
	decidedPayload.EditedChange = &editedChange
	return nil
}
