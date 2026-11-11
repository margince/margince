// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who made each promise a transcript states, in the terms the commitment rule
// asks for.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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
	c.SystemText = &systemtext.Message{Subject: systemtext.Prose(systemtext.Generated, ""), Body: transcriptCommitmentComponent(c.Party, step.SourceLines)}
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
	code, lines := transcriptCommitmentLocation(sourceLines)
	return fmt.Sprintf(systemtext.Copy(code, textlang.English), owner, lines)
}

func transcriptCommitmentLocation(sourceLines []int) (string, string) {
	lines := make([]string, len(sourceLines))
	for i, line := range sourceLines {
		lines[i] = strconv.Itoa(line)
	}
	code := "commitment.transcript_line"
	if len(lines) > 1 {
		code = "commitment.transcript_lines"
	}
	return code, strings.Join(lines, ", ")
}

func transcriptCommitmentComponent(owner string, sourceLines []int) *systemtext.Component {
	code, lines := transcriptCommitmentLocation(sourceLines)
	return systemtext.Sentence(code, systemtext.Text(owner), systemtext.Text(lines))
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
