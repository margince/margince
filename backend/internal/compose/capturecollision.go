// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/approvals"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
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

// captureCollisionActor is the provenance of the write.
//
// The values came from an inbound message, not from somebody typing them, so
// the write is stamped to this actor while the human's decision stays on the
// approval's own audit row. A later human edit must still outrank it.
const captureCollisionActor = "capture-collision"

// capturedLead is the proposal's payload: the incumbent this fold applies to,
// plus what the message knew.
//
// The field keys are Go field names rather than snake_case, and they have to
// stay that way: capture marshals its own `LeadFields` struct, which carries no
// json tags, so every approval already staged in every installation holds
// `FullName` and `CompanyName` on disk. Tagging the writer would read those
// pending rows as empty and fold nothing.
type capturedLead struct {
	LeadID ids.LeadID `json:"lead_id"`
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
// seam's: `FillEmptyLeadFieldsTx` compares against the row inside the same
// transaction it writes. Deciding it here would mean reading the lead first and
// patching after, and a concurrent edit landing between the two would let a
// captured value overwrite a value a person had just typed.
func captureCollisionAcceptEffect(svc *approvals.Service, store *people.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var captured capturedLead
		if err := json.Unmarshal(proposedChange, &captured); err != nil {
			return fmt.Errorf("compose: decoding the captured lead fields: %w", err)
		}
		if captured.LeadID.IsZero() {
			// A proposal staged before the payload carried the incumbent's id.
			// There is no record to fold onto, and guessing one from the target
			// column would be writing to a lead this effect never verified.
			return fmt.Errorf("compose: the capture-collision proposal names no lead")
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return fmt.Errorf("compose: capture-collision accept without a deciding principal")
		}
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         captureCollisionActor,
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		// The custom-field catalog is read BEFORE the transaction opens. Read
		// inside it, this would acquire a second connection within a
		// transaction it does not own — which commits separately and can
		// deadlock against a lock that transaction holds.
		active, err := store.ActiveLeadColumns(execCtx)
		if err != nil {
			return err
		}
		return svc.RedeemAndApply(ctx, approvalID, captureCollisionKind, diffHash, func(tx pgx.Tx) error {
			return store.FillEmptyLeadFieldsTx(execCtx, tx, captured.LeadID, people.CapturedLeadFields{
				FullName:    captured.FullName,
				Email:       captured.Email,
				CompanyName: captured.CompanyName,
				Title:       captured.Title,
			}, active)
		})
	}
}
