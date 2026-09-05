// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What confirming a transcript-derived next step actually does.
//
// Nothing until here: the reading staged a question and wrote no task, no deal
// change and no contact (GATE-AI-2). This is the only place a transcript
// proposal becomes a row, and it is the same RBAC-gated write a rep's own "add
// task" takes — the confirmation grants the act, it does not bypass the gate.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// transcriptProposalSourceSystem keys the created task to the decision that
// created it. Together with SourceID = the approval id it is the second of two
// independent idempotency claims (the first being the single-use redemption),
// so a re-driven decision creates nothing twice.
const transcriptProposalSourceSystem = "transcript-proposal"

// transcriptProposalActor is who the created task is captured by. The next step
// is the reader's suggestion, not the confirming human's own note — the human
// is on the decision's audit row, which is where "who approved this" belongs.
const transcriptProposalActor = "agent:transcript-proposer"

// transcriptProposalEffect executes an approved next step: redeem and create in
// ONE transaction, so the approval is spent if and only if the task exists.
//
// RedeemAndApply rather than Redeem-then-write, because the two-transaction
// shape has a hole with no way out of it. If the write fails — the meeting was
// relinked to a deal somebody has since archived, or any transient fault hits
// the link insert — the approval is already consumed: it cannot be decided
// again, cannot be redeemed again, and nothing else drives the effect. The task
// is lost and the rep is told only that "executing the effect failed". Sharing
// the transaction rolls the redemption back with the write, leaving the
// proposal exactly where it was, ready to be approved again.
//
// The write stays additionally idempotent on (source_system, source_id) keyed
// to the approval, which is what makes a REPLAY of the whole effect safe.
func transcriptProposalEffect(svc *approvals.Service, store *activities.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		proposal, err := UnmarshalTranscriptStepProposal(proposedChange)
		if err != nil {
			return err
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return fmt.Errorf("compose: transcript proposal effect without a deciding principal")
		}
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         transcriptProposalActor,
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		subject := proposal.Summary
		body := transcriptTaskBody(proposal)
		sourceSystem := transcriptProposalSourceSystem
		sourceID := approvalID.String()
		return svc.RedeemAndApply(ctx, approvalID, TranscriptProposalKind, diffHash, func(tx pgx.Tx) error {
			in := activities.LogActivityInput{
				Kind:         "task",
				Subject:      &subject,
				Body:         &body,
				SourceSystem: &sourceSystem,
				SourceID:     &sourceID,
				Source:       transcriptProposalSourceSystem,
				Links:        proposal.Links,
			}
			if err := stampTranscriptDue(ctx, tx, &in, proposal.DueDate); err != nil {
				return err
			}
			_, _, err := store.LogActivityTx(execCtx, tx, in)
			return err
		})
	}
}

// stampTranscriptDue puts the day a transcript stated onto the task, as the
// moment that day ENDS in the installation's own zone.
//
// End of day rather than its start, because a deadline is the last moment the
// thing is still on time: a task due "8 September" filed at that day's midnight
// is overdue for the whole of the 8th, which is not what anybody in the meeting
// agreed to. It is the same rule the composer's own date box applies when a rep
// types a due date by hand.
//
// The INSTALLATION's zone, not the decider's: a task's due day is a fact about
// the record, read back by colleagues in other places, and a deadline that fell
// on a different day depending on who opened it would be two deadlines.
//
// A day that will not parse is an ERROR rather than a silent skip: it means a
// reviewer edited the payload into something acceptance cannot read, and
// quietly creating an undated task would hide that from them.
func stampTranscriptDue(ctx context.Context, tx pgx.Tx, in *activities.LogActivityInput, day string) error {
	// An undated next step leaves DueAt unset, which is what "nobody said when"
	// looks like on the task. Expressed by not setting the field rather than by
	// answering a nil instant, so there is one reading of "no deadline" here
	// instead of two.
	if day == "" {
		return nil
	}
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return fmt.Errorf("compose: installation timezone %q: %w", zone, err)
	}
	parsed, err := time.ParseInLocation(time.DateOnly, day, loc)
	if err != nil {
		return fmt.Errorf(
			"compose: transcript proposal due date %q is not a date — write it as YYYY-MM-DD: %w",
			day, err)
	}
	// The last second of the named day, matching format/calendarday.dueInstant
	// on the web side so a deadline typed by hand and one read from a
	// transcript mean the same moment.
	due := parsed.Add(24*time.Hour - time.Second).UTC()
	in.DueAt = &due
	return nil
}

// transcriptTaskBody says where the task came from, in the terms the rep can
// go and check: whose commitment it was, and which lines of which transcript
// said so. The task outlives the approval it came from, so the provenance is
// written into it rather than left as a link to a row that may be swept.
func transcriptTaskBody(proposal TranscriptStepProposal) string {
	lines := make([]string, 0, len(proposal.SourceLines))
	for _, line := range proposal.SourceLines {
		lines = append(lines, strconv.Itoa(line))
	}
	where := "line " + strings.Join(lines, ", ")
	if len(lines) > 1 {
		where = "lines " + strings.Join(lines, ", ")
	}
	return fmt.Sprintf("%s committed to this in the meeting transcript (%s).", proposal.Owner, where)
}
