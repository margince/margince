// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MayReadMeetings is the rule for every figure counted from meetings. Weekly
// and the live Analytics reviews share it. A meeting is an activity, so the
// seat needs activity read.
func MayReadMeetings(ctx context.Context) bool {
	return auth.Allows(ctx, "activity", principal.ActionRead)
}

// withholdReviewMeetings takes the meeting figures out of a frozen rep week
// for a reader who may not read them. The narrative and the learnings go too.
// A model wrote them from those figures, and may have stated one. They read
// as never run, which is true for this reader.
func withholdReviewMeetings(review *Review) {
	review.MeetingsWithheld = true
	review.Counts.MeetingsHeld, review.Counts.MeetingsWithNextStep = 0, 0
	if review.Scorecard != nil && review.Scorecard.Lead != nil {
		lead := *review.Scorecard.Lead
		lead.Booked, lead.Held, lead.NoShow, lead.PartialHistory = 0, 0, 0, 0
		review.Scorecard.Lead = &lead
	}
	review.Narrative, review.NarratedAt = "", nil
	review.LearningsState, review.Learnings = LearningsNotRun, nil
}

// withholdTeamMeetings does the same for a frozen team week. The focus is
// always the first pattern. So a rep whose only pattern was meetings without a
// next step reads as a quiet week, in the snapshot's own language.
func withholdTeamMeetings(review *TeamReview) {
	review.MeetingsWithheld = true
	review.Counts.MeetingsHeld, review.Counts.MeetingsWithNextStep = 0, 0
	said := teamCopyFor(review.Language)
	for i := range review.Reps {
		rep := &review.Reps[i]
		rep.MeetingsHeld = 0
		rep.Patterns = slices.DeleteFunc(slices.Clone(rep.Patterns), func(p Pattern) bool {
			return p.Kind == FocusMeetingsWithoutNextStep
		})
		if len(rep.Patterns) == 0 {
			rep.Patterns = []Pattern{{Kind: FocusQuietWeek, Label: said.Say(patternWords.Quiet)}}
		}
		rep.FocusKind, rep.FocusLabel = rep.Patterns[0].Kind, rep.Patterns[0].Label
	}
}
