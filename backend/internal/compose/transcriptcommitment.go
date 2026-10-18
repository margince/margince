// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who made each promise a transcript states, in the terms the commitment rule
// asks for.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// transcriptProposalActor is who a commitment read out of a transcript is
// captured by: the reader's, not the confirming human's own note. The human is
// on the decision's audit row, which is where "who approved this" belongs.
const transcriptProposalActor = "agent:transcript-proposer"

// commitmentFrom rechecks the quoted promiser against the meeting participants.
func (p *TranscriptProposer) commitmentFrom(
	ctx context.Context, tx pgx.Tx, step proposedStep,
	reading activities.TranscriptReading, activityID ids.ActivityID, asOf time.Time,
) (Commitment, error) {
	cited := quotedFromTranscript(step, reading.Lines)
	owner := strings.TrimSpace(step.Owner)
	c := Commitment{
		AsOf:             asOf,
		Extractor:        transcriptProposalActor,
		SourceActivityID: activityID.UUID,
		ReadMessages:     []ids.UUID{activityID.UUID},
		Summary:          step.Summary,
		Quote:            cited,
		Party:            owner,
		DueDate:          step.DueDate,
		Confidence:       float64(step.Confidence),
		Links:            reading.Links,
		Evidence:         stepEvidence(step, reading.Lines, activityID),
	}
	reader := extractorContext(ctx, transcriptProposalActor)
	parties, err := p.messages.PartiesOf(reader, tx, activityID.UUID)
	if err != nil {
		return Commitment{}, err
	}
	verified := verifiedCommitmentParties(parties)
	kind, party := attributeCommitment(&c, step.PromiserRef, verified)
	c.NeedsConfirmation = !transcriptPromiserSpoke(step, reading.Lines, verified)
	c.Body = transcriptCommitmentBody(c.Party, step.SourceLines)
	// The first cited line is part of the span: the same words said twice in
	// one meeting are two promises.
	span := cited
	if len(step.SourceLines) > 0 {
		span = strconv.Itoa(step.SourceLines[0]) + ":" + cited
	}
	c.Span = span
	c.Locator = commitmentLocator(activityID.UUID, kind, party, span)
	return c, nil
}

// linkedContacts is the contacts a meeting is filed against.
func linkedContacts(links []activities.ActivityLinkInput) []ids.UUID {
	var out []ids.UUID
	for _, link := range links {
		if link.EntityType == string(recordTypeContact) {
			out = append(out, link.EntityID)
		}
	}
	return out
}

// transcriptCommitmentBody says where the task came from, in the terms a rep
// can go and check: whose commitment it was, and which lines said so. The task
// outlives any approval it came through, so the provenance is written into it.
func transcriptCommitmentBody(owner string, sourceLines []int) string {
	lines := make([]string, 0, len(sourceLines))
	for _, line := range sourceLines {
		lines = append(lines, strconv.Itoa(line))
	}
	where := "line " + strings.Join(lines, ", ")
	if len(lines) > 1 {
		where = "lines " + strings.Join(lines, ", ")
	}
	return fmt.Sprintf("Promiser: %s. Commitment in the meeting transcript (%s).", owner, where)
}

// transcriptReadDetail says what a reading produced besides its questions. A
// run that finishes with nothing and no reason reads exactly like a broken
// one, which FinishTranscriptRead refuses to let collapse, and a task written
// without asking is said here because no question shows it.
func transcriptReadDetail(found int, staged transcriptStaging) string {
	var details []string
	if staged.tasks > 0 {
		details = append(details, "added "+counted(staged.tasks, "task for a colleague", "tasks for colleagues"))
	}
	if staged.watched > 0 {
		details = append(details, "noted "+counted(staged.watched, "commitment the customer made", "commitments the customer made"))
	}
	if len(staged.proposals) > 0 {
		details = append(details, counted(len(staged.proposals), "proposal awaits your decision", "proposals await your decision"))
	}
	for _, reason := range slices.Sorted(maps.Keys(staged.withheld)) {
		details = append(details, counted(staged.withheld[reason], "commitment", "commitments")+": "+transcriptWithheldReason(reason))
	}
	if len(details) > 0 {
		return strings.Join(details, "; ")
	}
	if found == 0 {
		return "this transcript states no next steps clearly enough to propose one"
	}
	return "every next step this transcript states has already been put to you"
}

// counted spells a count with the noun in the right number.
func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func transcriptWithheldReason(outcome CommitmentOutcome) string {
	switch activities.CommitmentAdmissionReason(outcome) {
	case activities.CommitmentHistorical:
		return "kept as history without creating new work"
	case activities.CommitmentIncomplete, activities.CommitmentContextUnassessed:
		return "needs later conversation evidence before creating work"
	case activities.CommitmentSourceUnknown:
		return "needs a verified source date before creating work"
	case activities.CommitmentTimeContradicted:
		return "has conflicting message dates; review the source before creating work"
	case activities.CommitmentMeetingCancelled:
		return "belongs to a canceled meeting; no work created"
	case activities.CommitmentMeetingPending:
		return "belongs to a meeting that has not taken place; no work created"
	default:
		return "source is no longer available; no work created"
	}
}

func transcriptPromiserSpoke(step proposedStep, lines []string, parties []commitmentParty) bool {
	promiser, found := referencedPromiser(step.PromiserRef, parties)
	if !found || strings.TrimSpace(promiser.Name) == "" {
		return false
	}
	for _, number := range step.SourceLines {
		if number < 1 || number > len(lines) {
			continue
		}
		speaker, _, labeled := strings.Cut(lines[number-1], ":")
		if labeled && strings.EqualFold(strings.TrimSpace(speaker), strings.TrimSpace(promiser.Name)) {
			return true
		}
	}
	return false
}
