// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The coaching layer, for a lead reading a teammate's meeting.
//
// TWO PROPERTIES HOLD THIS TOGETHER, and they are the whole design.
//
// It adds a READING and never a fact. The base plan is built once, blind to who
// is reading it, and coaching is attached over that finished plan — so a lead
// and their rep are looking at the same meeting, and the lead sees one more
// thing. Generating a second plan for the lead would let the two drift, and a
// lead coaching from facts their rep cannot see is coaching about a different
// meeting.
//
// Who gets it is the SERVER's decision. No client asks for it, because a client
// that could ask could ask on anybody's behalf. The rule is the one the tree
// already uses for raising a coaching notice: a seat that may coach at all, and
// a live team shared with somebody in the room.

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Teammates answers whether one colleague is on a live team with the caller.
//
// The caller is not a parameter: the module behind this reads it from the
// principal, so a lead cannot ask about an edge they are not an end of. It is
// the same shape `notices.Teammates` declares and the same seam compose binds
// for the Worklist — one answer to "are these two teammates", not two.
type Teammates interface {
	SharesLiveTeamWithCaller(ctx context.Context, other ids.UUID) (bool, error)
}

// WithTeammates binds the membership reader. A service without one projects no
// coaching, which is the honest answer for a composition that did not wire it.
func (s *Service) WithTeammates(mates Teammates) *Service {
	s.teammates = mates
	return s
}

// coachingProjected decides whether this reader gets the coaching layer.
//
// Two questions in the order notices.RaiseCoachNotice asks them. May this SEAT
// coach at all — a role question, refusing an agent, a buyer and a rep alike.
// Then: is there anybody in this room to coach — a live team shared with
// somebody OTHER than the reader.
//
// Being seated yourself is not a disqualifier. A lead in the room coaching
// their rep through it is the ordinary case, and excluding them would be a
// carve-out nobody asked for.
func (s *Service) coachingProjected(ctx context.Context, seats []ids.UUID) (bool, error) {
	if s.teammates == nil {
		return false, nil
	}
	if err := auth.RequireCoach(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			// THE RULE, not a swallowed error. A rep, an agent and a buyer all
			// land here, and for all three the answer is "you get the rep's
			// brief" rather than a refusal — they asked for a brief and a brief
			// is what they may have.
			return false, nil
		}
		return false, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		// RequireCoach admits only a human, so this is unreachable through the
		// gate above. Refusing rather than guessing: a coaching decision with
		// no identity behind it is one nobody can account for.
		return false, nil
	}
	for _, seat := range seats {
		if seat == ids.UUID(actor.UserID) {
			continue
		}
		shares, err := s.teammates.SharesLiveTeamWithCaller(ctx, seat)
		if err != nil {
			// Not swallowed. A membership check that is BROKEN must not answer
			// "no coaching": that is indistinguishable from a correct denial,
			// and a lead would quietly stop being coached with nothing to see.
			return false, fmt.Errorf("meeting brief: reading team membership for coaching: %w", err)
		}
		if shares {
			return true, nil
		}
	}
	return false, nil
}

// coachingFor reads the finished plan and says how this meeting could go wrong.
//
// It takes the plan rather than the Input on purpose: everything it says is a
// reading of what the rep is already being told, so there is no path by which
// it could introduce a fact the rep's own brief does not carry.
func coachingFor(plan Plan, in Input) *Coaching {
	coaching := &Coaching{
		Focus:       coachingFocus(plan),
		FailureMode: coachingFailureMode(plan, in),
		ListenFor:   "A quantified consequence in their words, and who owns it today.",
		WatchFor:    coachingWatchFor(plan),
		InterveneIf: "A date, a price or a resource is promised that nobody on our side has agreed.",
		Paths:       coachingPaths(plan),
	}
	return coaching
}

// Coaching is the layer, before it reaches the wire.
type Coaching struct {
	Focus       string
	FailureMode string
	ListenFor   string
	WatchFor    string
	InterveneIf string
	Paths       []CoachingPath
}

// CoachingPath is one way the meeting can go.
type CoachingPath struct {
	Label string
	Play  string
}

func coachingFocus(plan Plan) string {
	if plan.Objective != nil {
		return "Whether the rep leaves with the outcome the plan names, or settles for a pleasant conversation."
	}
	return "Whether the rep establishes what this meeting is for before spending it."
}

// coachingFailureMode names the likeliest way THIS meeting goes wrong, read off
// what the plan already found.
func coachingFailureMode(plan Plan, in Input) string {
	switch {
	case plan.TopRisk != nil:
		return "Defending the history instead of owning it and naming a date."
	case len(plan.LikelyAsks) >= 2:
		return "Answering the asks one by one and never getting to a question of their own."
	case plan.Type.Value == crmcontracts.MeetingPlanTypeUnknown:
		return "Assuming what the meeting is for, and finding out at the end that it was not."
	case len(in.Commitments) == 0:
		return "Filling the silence with product rather than asking what is actually going on."
	default:
		return "Proving coverage too early, before the problem is agreed."
	}
}

func coachingWatchFor(plan Plan) string {
	if len(plan.Questions) > 0 {
		return "The rep answering before asking. Count the questions they get to."
	}
	return "A conversation with no question in it."
}

// coachingPaths are the branches a lead rehearses against. Taken from the
// plan's own scenarios where it has them, so the lead and the rep are
// preparing for the same meeting.
func coachingPaths(plan Plan) []CoachingPath {
	paths := make([]CoachingPath, 0, len(plan.Scenarios))
	for _, scenario := range plan.Scenarios {
		paths = append(paths, CoachingPath{Label: scenario.Label, Play: scenario.Play})
	}
	return paths
}

// wireCoaching renders the layer. It carries no citations of its own: every
// fact it rests on is in the plan beside it, and inventing a citation here
// would claim a record supports a reading of the REP rather than of the account.
func wireCoaching(coaching *Coaching) *crmcontracts.MeetingPlanCoaching {
	if coaching == nil {
		return nil
	}
	paths := make([]crmcontracts.MeetingPlanCoachingPath, 0, len(coaching.Paths))
	for _, path := range coaching.Paths {
		paths = append(paths, crmcontracts.MeetingPlanCoachingPath{
			Label: path.Label, Play: path.Play,
		})
	}
	return &crmcontracts.MeetingPlanCoaching{
		Focus:       coaching.Focus,
		FailureMode: coaching.FailureMode,
		ListenFor:   coaching.ListenFor,
		WatchFor:    coaching.WatchFor,
		InterveneIf: coaching.InterveneIf,
		Paths:       paths,
	}
}
