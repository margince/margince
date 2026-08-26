// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// What happens when a human accepts a captured record that collided with one
// already here.
//
// The card is staged by capture when an inbound record matches a lead that
// exists: the capture writes NOTHING, and the proposal carries the incumbent
// plus the fields the message would have set. Despite the kind's name there is
// no second record and no merge — the second was never created, so there is
// nothing to merge into the first. What the reader is offered is a fold: take
// what the message knew, and fill what the lead does not have.
//
// Until this effect existed the kind had no entry in the registry, so
// `decide.go` ran nothing: accepting marked the approval approved, minted a
// token, and left the record untouched. The card asked a real question and
// discarded the answer.

// captureCollisionKind is the staged kind. It is spelled `merge_records`
// because that is what capture has always staged and what every pending row in
// every installation already carries; renaming it would strand them.
const captureCollisionKind = "merge_records"

// captureCollisionTarget is the only entity type capture stages this kind
// against. Checked rather than assumed: the target columns decide where this
// writes, so a row naming something else is a write nobody proposed.
const captureCollisionTarget = "lead"

// capturedLead is the proposal's payload: what the message knew.
//
// It carries no record id, and must not. The lead this folds onto comes from
// the approval's own target columns, which are NOT editable — the payload is
// what the deciding human sees and may correct, so an id carried there would
// let an approver retarget the write onto a record they were never shown.
// `StagedTarget` is the API built for exactly this, and reading it back is
// what binds the row written to the target the decision grants were checked
// against.
//
// The keys are capture's Go field names, not snake_case: capture marshals its
// own tagless `LeadFields`, so every approval already staged in every
// installation holds them on disk. Tagging the writer would read those pending
// rows as empty and fold nothing.
type capturedLead struct {
	//nolint:tagliatelle // the keys are capture's Go field names, already on disk in every pending row
	FullName string `json:"FullName"`
	//nolint:tagliatelle // as above
	Email string `json:"Email"`
	//nolint:tagliatelle // as above
	CompanyName string `json:"CompanyName"`
	//nolint:tagliatelle // as above
	Title string `json:"Title"`
}

// captureCollisionAcceptEffect folds the captured fields onto the lead they
// collided with.
//
// Which fields actually change is the people module's decision, not this
// seam's: `FillEmptyLeadFieldsTx` locks the row and compares against it inside
// the same transaction it writes. Deciding it here would mean reading the lead
// first and patching after, and an edit landing between the two would let a
// captured value overwrite a value a person had just typed.
//
// The write runs under the DECIDING HUMAN's own principal. Swapping in a system
// principal would bypass object RBAC and unbound row scope — so an ownership
// change landing between the decision's authority check and this write would be
// ignored, and the effect would write where the person no longer may. Machine
// provenance belongs on the audit row, not in the authorization principal.
func captureCollisionAcceptEffect(svc *approvals.Service, store *people.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var captured capturedLead
		if err := json.Unmarshal(proposedChange, &captured); err != nil {
			return fmt.Errorf("compose: decoding the captured lead fields: %w", err)
		}
		if _, ok := principal.Actor(ctx); !ok {
			return fmt.Errorf("compose: capture-collision accept without a deciding principal")
		}
		entityType, entityID, err := svc.StagedTarget(ctx, approvalID)
		if err != nil {
			return err
		}
		if entityType != captureCollisionTarget {
			// Capture stages this kind against a lead and nothing else. A row
			// naming another type is not a collision this effect can fold, and
			// writing to it would be writing somewhere nobody proposed.
			return fmt.Errorf("compose: capture-collision names a %s target, not a lead", entityType)
		}
		// The custom-field catalog is read BEFORE the transaction opens. Read
		// inside it, this would acquire a second connection within a
		// transaction it does not own — which commits separately and can
		// deadlock against a lock that transaction holds.
		active, err := store.ActiveLeadColumns(ctx)
		if err != nil {
			return err
		}
		return svc.RedeemAndApply(ctx, approvalID, captureCollisionKind, diffHash, func(tx pgx.Tx) error {
			return store.FillEmptyLeadFieldsTx(ctx, tx, ids.From[ids.LeadKind](entityID), people.CapturedLeadFields{
				FullName:    captured.FullName,
				Email:       captured.Email,
				CompanyName: captured.CompanyName,
				Title:       captured.Title,
			}, active)
		})
	}
}
