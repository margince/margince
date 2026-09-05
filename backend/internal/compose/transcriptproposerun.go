// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What one reading of one transcript does, from claiming the run record to
// closing it.
//
// The division of labour with transcriptpropose.go is: that file owns the
// question put to the model and what may come back, this one owns the run — the
// claim, the staging, and the three outcomes a rep can be shown. Keeping the
// outcomes here is deliberate, because the difference between them is the
// product: "still reading", "read it and it stated nothing", and "could not
// read it" must never collapse into one another.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TranscriptProposalKind is the staging kind a transcript reading produces. It
// is registered with an executor in coldstartaccept.go and with a decision
// grant in the approvals module; a kind missing either is one no human can
// decide, which the composition root's fitness tests refuse.
const TranscriptProposalKind = "transcript_proposal"

// transcriptTargetType is what the proposal is filed against: the transcript it
// was read from. The proposal does not MODIFY that activity — it proposes a new
// one — which is why the kind is a context target in approvals rather than a
// version-pinned one.
const transcriptTargetType = string(recordTypeActivity)

// TranscriptStepProposal is the staged payload: what was promised, by whom, and
// where in the transcript it was said.
//
// The links are frozen at staging time rather than re-read on accept. A rep
// confirms the proposal they were shown, and re-reading the transcript's links
// at accept would let a relink between the two moments silently move where the
// task lands.
type TranscriptStepProposal struct {
	ActivityID ids.UUID `json:"activity_id"`
	Summary    string   `json:"summary"`
	Owner      string   `json:"owner"`
	// DueDate is the day the transcript stated, as YYYY-MM-DD, or empty.
	//
	// Text rather than an instant, so the reviewer edits a DAY and acceptance
	// decides what moment that day ends at. A payload carrying an instant would
	// have fixed the zone at extraction, hours before anybody looked at it.
	//
	// Absent on a proposal staged before this field existed, which decodes to
	// empty — the same as "the transcript stated no deadline". Those two are
	// genuinely indistinguishable from the payload alone, and both produce a
	// task with no date, so nothing is lost by not telling them apart.
	DueDate     string                         `json:"due_date,omitempty"`
	SourceLines []int                          `json:"source_lines"`
	Links       []activities.ActivityLinkInput `json:"links"`
	// Cited is the transcript's OWN words behind this step, and it is what a
	// second reading of the same transcript has in common with the first.
	//
	// Nothing else here does. Summary and Owner are the model's prose and vary
	// between readings; SourceLines is the model's citation and can shift by a
	// line for the same sentence. The transcript is a fixed document, so the
	// commitment it states is the one thing that holds still — which is what the
	// rejection memory has to key on for a rep's "no" to survive somebody
	// pressing read again.
	//
	// It duplicates the evidence snippet deliberately. A staging identity must be
	// a string the PAYLOAD carries with the same value (canonicalIdentity
	// enforces both), and evidence is a sibling record rather than part of the
	// proposed change.
	Cited string `json:"cited"`
}

// The payload keys the staging identity is drawn from. Each must spell exactly
// what the struct tag above spells — canonicalIdentity refuses an identity field
// the payload does not carry, so a typo here is a staging that fails when a rep
// reads a transcript rather than at compile time.
const (
	transcriptIdentityCited = "cited"
	transcriptIdentityOwner = "owner"
)

// UnmarshalTranscriptStepProposal reads back what was staged.
func UnmarshalTranscriptStepProposal(raw json.RawMessage) (TranscriptStepProposal, error) {
	var out TranscriptStepProposal
	if err := json.Unmarshal(raw, &out); err != nil {
		return TranscriptStepProposal{}, fmt.Errorf("compose: unmarshal transcript step proposal: %w", err)
	}
	return out, nil
}

// Read performs one reading: claim the run, put the transcript to the model,
// stage what it says was promised, and close the run with what happened.
//
// It returns an error only for a fault the JOB should retry — the model lane
// being down, the database being unreachable. A reading that legitimately
// could not be used closes the run as failed and returns nil, because retrying
// would ask the same question of the same text and get the same answer.
func (p *TranscriptProposer) Read(ctx context.Context, store transcriptReadStore, readID ids.UUID, activityID ids.ActivityID) error {
	if _, err := store.BeginTranscriptRead(ctx, readID, activities.TranscriptReadLease); err != nil {
		return err
	}
	reading, err := store.ReadTranscript(ctx, activityID)
	if err != nil {
		if detail, terminal := unreadableTranscript(err); terminal {
			return p.fail(ctx, store, readID, detail)
		}
		// Anything else — the database unreachable, a scoped transient fault —
		// is the JOB's to retry. Closing the reading here would turn a blip
		// into a permanent verdict the rep has to notice and undo.
		return err
	}
	// Checked again even though the door checked it: the body can be edited
	// between the request and the reading, and a re-normalized transcript is a
	// different size than the one that was queued.
	if err := activities.WithinReadingBounds(reading.Lines); err != nil {
		return p.fail(ctx, store, readID, err.Error())
	}
	// The day the activity is FILED under, which is the best available answer
	// to "when was this conversation". The composer offers it as an editable
	// date capped at today, so a rep pasting a three-week-old transcript can
	// set the day it happened — and it defaults to today, so one who does not
	// leaves the paste day standing.
	//
	// Said plainly because the difference matters: a relative deadline resolves
	// against whatever this says, so "by Friday" on a backdated transcript
	// whose date was left at today resolves to the wrong week. The reviewer
	// sees the resulting date on the card before any task exists, which is
	// where that is caught.
	steps, err := p.ask(ctx, reading.Lines, reading.OccurredAt.Format(time.DateOnly))
	if err != nil {
		if errors.Is(err, errRefusedTranscript) {
			p.log.WarnContext(ctx, "transcript reading refused",
				"transcript_read_id", readID, "activity_id", activityID, "reason", err)
			return p.fail(ctx, store, readID,
				"the model's reading of this transcript could not be used; the transcript is unchanged and can be read again")
		}
		return err
	}
	kept := aboveFloor(steps)
	if len(kept) == 0 {
		return store.FinishTranscriptRead(ctx, readID, activities.TranscriptReadOutcome{
			Status:    activities.TranscriptReadDone,
			Detail:    "this transcript states no next steps clearly enough to propose one",
			LineCount: len(reading.Lines),
		})
	}
	staged, err := p.stage(ctx, kept, reading, activityID)
	if err != nil {
		return err
	}
	outcome := activities.TranscriptReadOutcome{
		Status:      activities.TranscriptReadDone,
		ProposalIDs: staged,
		LineCount:   len(reading.Lines),
	}
	if len(staged) == 0 {
		// The reading found commitments and raised none of them: every one was
		// already answered, either waiting in the queue or turned down before.
		// That is a real outcome and it needs saying — a run that finishes with
		// nothing and no reason reads exactly like a broken one, which is the
		// distinction FinishTranscriptRead refuses to let collapse.
		outcome.Detail = "every next step this transcript states has already been put to you"
	}
	return store.FinishTranscriptRead(ctx, readID, outcome)
}

// unreadableTranscript separates a refusal a rep can act on from a fault the
// job should retry, and answers with the message the rep is shown.
//
// Only the typed refusals reach status_detail. A raw err.Error() there would
// put a driver string ("failed to connect to host=…") in front of a rep on the
// one field this feature exists to make readable, and would settle a transient
// blip as a permanent failure.
func unreadableTranscript(err error) (detail string, terminal bool) {
	var notTranscript *activities.NotATranscriptError
	var tooLong *activities.TranscriptTooLongError
	switch {
	case errors.As(err, &notTranscript), errors.As(err, &tooLong):
		return err.Error(), true
	case errors.Is(err, activities.ErrBlankTranscript):
		return "this transcript is empty, so there is nothing to read", true
	case errors.Is(err, apperrors.ErrNotFound):
		return "this transcript is no longer available to read", true
	}
	return "", false
}

// fail closes the run with a reason a rep can act on. A failure to record the
// failure is returned, so a run cannot be left claimed and silent.
func (p *TranscriptProposer) fail(ctx context.Context, store transcriptReadStore, readID ids.UUID, detail string) error {
	return store.FinishTranscriptRead(ctx, readID, activities.TranscriptReadOutcome{
		Status: activities.TranscriptReadFailed,
		Detail: detail,
	})
}

// stage files each next step as its own question, under one bundle id.
//
// One bundle because they were asked together: a meeting that produced three
// commitments is one act of reading, and an inbox showing them as three
// unrelated questions makes the rep reconstruct that themselves (0200). Each
// still keeps its own diff hash, expiry and verdict — accepting two and
// rejecting one is the whole point.
func (p *TranscriptProposer) stage(
	ctx context.Context, steps []proposedStep, reading activities.TranscriptReading, activityID ids.ActivityID,
) ([]ids.UUID, error) {
	bundleID := ids.NewV7()
	staged := make([]ids.UUID, 0, len(steps))
	for _, step := range steps {
		proposal := TranscriptStepProposal{
			ActivityID:  activityID.UUID,
			Summary:     step.Summary,
			Owner:       step.Owner,
			DueDate:     step.DueDate,
			SourceLines: step.SourceLines,
			Links:       reading.Links,
			Cited:       quotedFromTranscript(step, reading.Lines),
		}
		raw, err := json.Marshal(proposal)
		if err != nil {
			return nil, fmt.Errorf("compose: marshal transcript step proposal: %w", err)
		}
		canonical, hash, err := diffhash.Canonical(raw)
		if err != nil {
			return nil, fmt.Errorf("compose: canonicalize transcript step proposal: %w", err)
		}
		// A rep can read the same transcript again — the in-flight uniqueness
		// index covers only a queued or running reading — so a refused step must
		// not come straight back. The identity is what the transcript itself
		// says: the words behind the step, and who it names as promising them.
		// Both hold still between readings of one document, where the model's
		// summary and its line citation do not — so a diff hash remembers
		// nothing.
		//
		// The owner is here to tell two commitments in ONE sentence apart. "I'll
		// send pricing and Dana will book the call" cites one line twice, and on
		// the quotation alone those are one identity: the second staging would
		// supersede the first and a rep would see one of the two, with nothing
		// saying the other existed. The owner is safe to key on for the same
		// reason the quotation is — it is the party AS THE TRANSCRIPT NAMES
		// THEM, carved out of the translation rule precisely because a
		// translated name is a different person.
		identity, err := json.Marshal(map[string]string{
			transcriptIdentityCited: proposal.Cited,
			transcriptIdentityOwner: proposal.Owner,
		})
		if err != nil {
			return nil, fmt.Errorf("compose: marshal transcript step identity: %w", err)
		}
		approvalID, staged1, err := p.approval.StageUnlessDeclined(ctx, approvals.StageInput{
			Kind:           TranscriptProposalKind,
			ProposedChange: canonical,
			DiffHash:       hash,
			Identity:       identity,
			TargetType:     transcriptTargetType,
			TargetID:       activityID.UUID,
			Summary:        step.Summary,
			Evidence:       []approvals.Evidence{stepEvidence(step, reading.Lines, activityID)},
			BundleID:       bundleID,
			JoinPending:    true,
		})
		if err != nil {
			return nil, err
		}
		if !staged1 {
			// Already refused, or already waiting. Either way this reading adds
			// nothing to the queue, and the run's own record says how many steps
			// it raised — so a step that was answered before is not counted again.
			continue
		}
		staged = append(staged, approvalID.UUID)
	}
	return staged, nil
}
