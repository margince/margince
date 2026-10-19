// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The coaching layer, for a lead reading a teammate's meeting.
//
// WHAT THIS LAYER IS, stated exactly, because the loose version of it is wrong.
//
// Coaching introduces NO NEW READ. The brief a lead gets is the brief that lead
// would have got anyway — assembled under their own grants, their own row scope
// and their own baseline — with one more object attached to it. Nothing here
// widens what they may see, and nothing here is generated from a second, richer
// assembly.
//
// It is NOT true that a lead and their rep see the same brief. The whole
// surface is caller-scoped on purpose: `readLastSpoke` keys "since you last
// spoke" on the reader's own id, the history is filtered through the reader's
// activity scope, and a team-scoped lead reaches rows an own-scoped rep does
// not. Two readers reading one meeting get two briefs, and that was true before
// this file existed. Claiming otherwise would be claiming this layer flattens a
// difference it does not touch.
//
// What IS held: coaching is attached over the finished plan rather than
// generated beside it, so the lead's coaching is a reading of the lead's own
// brief. A second assembly for the lead would let the two drift apart — the
// coaching would start describing a meeting the plan under it did not.
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
		// A principal that cleared RequireCoach and carries no seated identity
		// is malformed: the role keys came from somewhere, and the user id
		// should have come from the same place. It FAILS rather than reading as
		// "not coached", because a broken authenticated identity that answers
		// like an ordinary rep is one nobody ever looks into.
		//
		// notices.RaiseCoachNotice refuses the same shape for the same reason.
		return false, fmt.Errorf(
			"meeting brief: a principal cleared the coaching gate with no seated user: %w",
			apperrors.ErrPermissionDenied)
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
// The PLAN, and nothing else. It used to take the Input beside it and read one
// field off it, which is one field more than it needs and one more than it can
// justify: everything coaching says is a reading of what this reader is already
// being told, so the thing it reads should be exactly that. A coaching layer
// with its own source could say something the brief under it does not support.
func (said briefCopy) coachingFor(plan Plan) *Coaching {
	coaching := &Coaching{
		Focus:       said.coachingFocus(plan),
		FailureMode: said.coachingFailureMode(plan),
		ListenFor:   said.Say(coachingWords.Listen),
		WatchFor:    said.coachingWatchFor(plan),
		InterveneIf: said.Say(coachingWords.Intervene),
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

func (said briefCopy) coachingFocus(plan Plan) string {
	if plan.Objective != nil {
		return said.Say(coachingWords.Outcome)
	}
	return said.Say(coachingWords.Purpose)
}

// coachingFailureMode names the likeliest way THIS meeting goes wrong, read off
// what the plan already found.
func (said briefCopy) coachingFailureMode(plan Plan) string {
	switch {
	case plan.TopRisk != nil:
		return said.Say(coachingWords.History)
	case len(plan.LikelyAsks) >= 2:
		return said.Say(coachingWords.Asks)
	case plan.Type.Value == crmcontracts.MeetingPlanTypeValueMeetingPlanTypeUnknown:
		return said.Say(coachingWords.Assume)
	case len(plan.Arc) == 0:
		return said.Say(coachingWords.Silence)
	default:
		return said.Say(coachingWords.Coverage)
	}
}

func (said briefCopy) coachingWatchFor(plan Plan) string {
	if len(plan.Questions) > 0 {
		return said.Say(coachingWords.Questions)
	}
	return said.Say(coachingWords.NoQuestions)
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
