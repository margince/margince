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
		Meaning:          step.Meaning,
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
	if err := attributeCommitmentRecipients(&c, step.RecipientRefs, verified); err != nil {
		return Commitment{}, err
	}
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
func (said transcriptCopy) transcriptReadDetail(found int, staged transcriptStaging) string {
	var details []string
	if staged.tasks > 0 {
		details = append(details, said.count(staged.tasks, transcriptWords.TasksOne, transcriptWords.Tasks))
	}
	if staged.watched > 0 {
		details = append(details, said.count(staged.watched, transcriptWords.WatchedOne, transcriptWords.Watched))
	}
	if len(staged.proposals) > 0 {
		details = append(details, said.count(len(staged.proposals), transcriptWords.ProposalsOne, transcriptWords.Proposals))
	}
	for _, reason := range slices.Sorted(maps.Keys(staged.withheld)) {
		details = append(details, said.count(staged.withheld[reason], transcriptWords.WithheldOne, transcriptWords.Withheld, said.transcriptWithheldReason(reason)))
	}
	if len(details) > 0 {
		return strings.Join(details, "; ")
	}
	if found == 0 {
		return said.Say(transcriptWords.Empty)
	}
	return said.Say(transcriptWords.AlreadyKnown)
}

func (said transcriptCopy) transcriptWithheldReason(outcome CommitmentOutcome) string {
	switch outcome {
	case "semantic_completed":
		return said.Say(transcriptWords.Completed)
	case "semantic_conditional":
		return said.Say(transcriptWords.Conditional)
	case "semantic_optional":
		return said.Say(transcriptWords.Optional)
	case "semantic_request":
		return said.Say(transcriptWords.Request)
	case "semantic_unclear", "semantic_unassessed":
		return said.Say(transcriptWords.Unclear)
	case CommitmentPromiserUnassignable:
		return said.Say(transcriptWords.NoPromiser)
	}
	switch activities.CommitmentAdmissionReason(outcome) {
	case activities.CommitmentHistorical:
		return said.Say(transcriptWords.History)
	case activities.CommitmentIncomplete, activities.CommitmentContextUnassessed:
		return said.Say(transcriptWords.Incomplete)
	case activities.CommitmentSourceUnknown:
		return said.Say(transcriptWords.SourceUnknown)
	case activities.CommitmentTimeContradicted:
		return said.Say(transcriptWords.Contradicted)
	case activities.CommitmentMeetingCancelled:
		return said.Say(transcriptWords.Cancelled)
	case activities.CommitmentMeetingPending:
		return said.Say(transcriptWords.Pending)
	default:
		return said.Say(transcriptWords.SourceMissing)
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
