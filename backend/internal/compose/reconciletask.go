// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// overnightAgent captures the overnight pass's follow-up tasks, so a task
// reads as the agent's suggestion and not as something a rep typed.
const overnightAgent = "agent:overnight"

// writeOvernightFollowUp puts the follow-up task on the deal owner's list, when
// the owner still works here.
//
// The source is the deal and the interaction the task answers. A later pass over
// that interaction gets the first task back, even one the rep finished or
// deleted. So a task they cleared is not written again. A new interaction is a
// new source, and gets a new task.
func writeOvernightFollowUp(
	sweepCtx context.Context, tx pgx.Tx, tasks *activities.Store, owner ids.UUID, proposal deals.FollowUpProposal,
) error {
	var live bool
	if err := tx.QueryRow(sweepCtx, `SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1 AND `+
		workforce.LiveHumanSQL("")+`)`, owner).Scan(&live); err != nil || !live {
		return err
	}
	metadata := systemtext.ForWrite(sweepCtx, proposal.SystemText, proposal.Subject, proposal.Body)
	in, err := followUpTaskInput(metadata, proposal, followUpSourceID(proposal))
	if err != nil {
		return err
	}
	assignee := ids.From[ids.UserKind](owner)
	in.AssigneeID = &assignee
	_, _, err = tasks.LogActivityTx(onBehalfOf(extractorContext(sweepCtx, overnightAgent), owner), tx, in)
	return err
}

// followUpSourceID names one follow-up by its deal and the interaction behind it.
func followUpSourceID(proposal deals.FollowUpProposal) string {
	return proposal.DealID.String() + "/" + proposal.EvidenceActivityID.String()
}

// followUpTaskInput is the task the overnight pass files on a deal. The pass
// writes it itself, or a rep approves a proposal staged for it earlier.
func followUpTaskInput(
	metadata *systemtext.Message, proposal deals.FollowUpProposal, sourceID string,
) (activities.LogActivityInput, error) {
	due, err := time.Parse(time.DateOnly, proposal.DueDate)
	if err != nil {
		return activities.LogActivityInput{}, fmt.Errorf("compose: follow-up due date: %w", err)
	}
	subject, body := proposal.Subject, proposal.Body
	sourceSystem := provenance.OvernightReconcileSource
	return activities.LogActivityInput{
		Kind:         string(crmcontracts.ActivityKindTask),
		SystemText:   metadata,
		Subject:      &subject,
		Body:         &body,
		DueAt:        &due,
		SourceSystem: &sourceSystem,
		SourceID:     &sourceID,
		Source:       provenance.OvernightReconcileSource,
		Links:        []activities.ActivityLinkInput{{EntityType: "deal", EntityID: proposal.DealID.UUID}},
	}, nil
}
