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

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// commitmentFrom names who made one promise a transcript states.
//
// The owner arrives as PROSE — "Lena Fischer", "Frédéric", "the team" —
// because that is what a transcript states. A colleague is matched strictly
// (identity.ResolveColleague): an exact name or email, one live seat, or
// nobody. Failing that, a contact linked to the meeting under exactly that name
// is the customer who promised. A name that is neither stays unnamed, and the
// rule proposes it rather than guessing whose it is.
func (p *TranscriptProposer) commitmentFrom(
	ctx context.Context, tx pgx.Tx, step proposedStep,
	reading activities.TranscriptReading, activityID ids.ActivityID,
) (Commitment, error) {
	cited := quotedFromTranscript(step, reading.Lines)
	owner := strings.TrimSpace(step.Owner)
	c := Commitment{
		Extractor:        transcriptProposalActor,
		SourceActivityID: activityID.UUID,
		Summary:          step.Summary,
		Quote:            cited,
		Party:            owner,
		DueDate:          step.DueDate,
		Confidence:       float64(step.Confidence),
		Links:            reading.Links,
		Body:             transcriptCommitmentBody(owner, step.SourceLines),
		Evidence:         stepEvidence(step, reading.Lines, activityID),
	}
	reader := extractorContext(ctx, transcriptProposalActor)
	kind, party := claimKindOurs, "name:"+strings.ToLower(owner)
	colleague, found, err := p.users.ResolveColleague(reader, owner)
	if err != nil {
		return Commitment{}, err
	}
	linked := linkedContacts(reading.Links)
	if found {
		seat := colleague.UserID
		c.Seat, party = &seat, "seat:"+seat.String()
	} else {
		contact, named, err := p.contacts.ContactNamedAmong(reader, tx, linked, owner)
		if err != nil {
			return Commitment{}, err
		}
		if named {
			c.Theirs, kind, party = &contact, claimKindTheirs, "contact:"+contact.String()
		}
	}
	// One of our promises is filed on the customer's record too, when the
	// meeting names exactly one; with several, nothing says which was promised.
	if c.Theirs == nil && len(linked) == 1 {
		promisedTo := ids.From[ids.ContactKind](linked[0])
		c.PromisedTo = &promisedTo
	}
	c.Locator = commitmentLocator(activityID.UUID, kind, party, cited)
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
	return fmt.Sprintf("%s committed to this in the meeting transcript (%s).", owner, where)
}

// transcriptReadDetail says what a reading produced when it raised no
// question. A run that finishes with nothing and no reason reads exactly like
// a broken one, which FinishTranscriptRead refuses to let collapse.
func transcriptReadDetail(found int, staged transcriptStaging) string {
	switch {
	case found == 0:
		return "this transcript states no next steps clearly enough to propose one"
	case len(staged.proposals) > 0:
		return ""
	case staged.tasks > 0 || staged.watched > 0:
		return fmt.Sprintf("added %d task(s) for colleagues and noted %d promise(s) the customer made",
			staged.tasks, staged.watched)
	default:
		return "every next step this transcript states has already been put to you"
	}
}
