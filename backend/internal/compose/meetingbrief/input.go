// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// What one brief is written from.
//
// Nothing here re-queries. Every field is folded out of the meeting read and
// the composite 360 the caller already made, which is what makes the brief's
// scope exactly the reader's own scope without a second set of gates to keep in
// agreement.

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/personcontext"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Input is the room, the deal, what was promised, and what recently happened.
type Input struct {
	ActivityID string
	Subject    string
	StartsAt   time.Time
	// Now is the instant the brief was assembled, carried so "days since last
	// touch" is arithmetic on one clock rather than on time.Now() called from
	// several places that could straddle a midnight.
	Now time.Time

	Company string
	Deal    *DealIn
	// Project is the body of work the meeting belongs to, when it is filed
	// under one. It is what gives a DELIVERY meeting a goal: months after
	// close-won there is often no open deal, and the section that leads the
	// brief fell silent exactly when the engagement was the whole point.
	Project   *ProjectIn
	Attendees []AttendeeIn
	// Commitments are the room's open promises and questions, ours and theirs,
	// flattened across attendees because the reader asks "what is outstanding
	// with these people", not "what is outstanding with each of them".
	Commitments []ClaimIn
	// Recent is the newest captured conversation with the lead attendee first.
	Recent []ActIn
	// PriorMeetings are the last times this same room met, newest first. It is
	// what a recurring delivery review opens wanting: not the state of play,
	// but what was agreed here last time.
	PriorMeetings []PriorMeetingIn
	// LastSpokeAt is when the READER last dealt with anyone in the room — the
	// baseline "what changed" is measured from. Nil is first contact.
	LastSpokeAt *time.Time
	// LastTouchAt is the newest conversation with anyone in the room before
	// this meeting. Nil means nothing was ever captured with any of them.
	LastTouchAt *time.Time
	// DealMoves are what happened to the DEAL since LastSpokeAt — stage moves,
	// offers issued, what the buyer did in the Deal Room. Empty when nothing
	// moved, when the meeting is about no deal, or when this reader may not
	// see the part that moved.
	DealMoves []DealMoveIn
	// History is the room's whole readable correspondence within the last year,
	// as metadata: what happened and when, not what it said. It is what the
	// account arc is built from, and it is deliberately NOT in the prompt —
	// `json:"-"` — because two hundred rows would spend the model's budget on
	// what the arc has already condensed into five moments.
	History []HistoryIn `json:"-"`
	// Excerpts are the bodies of the few conversations the arc says matter,
	// bounded and audience-gated. Also out of the sections prompt: the plan is
	// what reads them.
	Excerpts []ExcerptIn `json:"-"`
	// Seats are the COLLEAGUES in this room, as against the attendees, who are
	// the counterparty. The coaching projection reads them to ask whether this
	// reader is a lead looking at somebody else's meeting. Out of the prompt:
	// who on our side is in the room is not a fact about the account.
	Seats []ids.UUID `json:"-"`
	// RoomHidden says this reader holds no deal_room grant, so whatever the
	// buyer did in the room is missing from DealMoves. Rendered as an omission
	// rather than left silent: a brief that cannot see the room reads exactly
	// like a brief about a deal with no room.
	RoomHidden bool
}

// DealIn is the deal this meeting is about.
type DealIn struct {
	ID          string
	Name        string
	Stage       string
	AmountMinor int64
	Currency    string
	CloseDate   *time.Time
}

// PriorMeetingIn is one earlier meeting with somebody from this room.
type PriorMeetingIn struct {
	ID       string
	Subject  string
	StartsAt time.Time
}

// foldPriorMeetings maps the read rows into the brief's own shape.
func foldPriorMeetings(rows []priorMeeting) []PriorMeetingIn {
	out := make([]PriorMeetingIn, 0, len(rows))
	for _, row := range rows {
		out = append(out, PriorMeetingIn{
			ID: row.ID.String(), Subject: row.Subject, StartsAt: row.StartsAt,
		})
	}
	return out
}

// ProjectIn is the engagement this meeting is part of.
type ProjectIn struct {
	ID            string
	Name          string
	Key           string
	Phase         string
	TargetEndDate *time.Time
}

// AttendeeIn is one person in the room.
type AttendeeIn struct {
	PersonID  string
	FullName  string
	Title     string
	DealRole  string
	LastTouch *time.Time
	FirstTime bool
}

// ClaimIn is one thing somebody in the room said. The kind rides along because
// "they promised to send it" and "we promised to send it" are opposite
// obligations, and the body alone loses which one it was.
type ClaimIn struct {
	PersonName string
	Kind       string
	Body       string
	Status     string
	SourceID   string
	// SourceLabel names the conversation in prose, so a sentence can say where
	// the promise was made without pasting a record id into the text.
	SourceLabel string
	DueAt       *time.Time
	// OccurredAt is when the claim was made, for ranking the newer first.
	OccurredAt *time.Time
}

// ActIn is one recent conversation as the brief reads it. Every field here is
// marshalled straight into the model's prompt (encodeInput), so this struct
// carries only what a reader may already see — WithCounterpart is where a
// conversation outside this caller's audience is kept out, not a flag on it
// here that the prompt would then have to be trusted to honour.
type ActIn struct {
	ID        string
	Kind      string
	Subject   string
	Direction string
	At        time.Time
}

// FromMeeting folds the gated meeting read into the brief's input.
func FromMeeting(room meeting, perAttendee map[ids.UUID][]crmcontracts.ConversationClaim, now time.Time) Input {
	in := Input{
		ActivityID: room.ID.String(),
		Subject:    room.Subject,
		StartsAt:   room.StartsAt.UTC(),
		Now:        now,
	}
	if room.Deal != nil {
		in.Deal = &DealIn{
			ID:          room.Deal.ID.String(),
			Name:        room.Deal.Name,
			Stage:       room.Deal.Stage,
			Currency:    room.Deal.Currency,
			CloseDate:   room.Deal.CloseDate,
			AmountMinor: derefInt(room.Deal.AmountMinor),
		}
	}
	if room.Project != nil {
		in.Project = &ProjectIn{
			ID:            room.Project.ID.String(),
			Name:          room.Project.Name,
			Key:           room.Project.Key,
			Phase:         room.Project.Phase,
			TargetEndDate: room.Project.TargetEndDate,
		}
	}
	for _, attendee := range room.Attendees {
		in.Attendees = append(in.Attendees, AttendeeIn{
			PersonID:  attendee.PersonID.String(),
			FullName:  attendee.FullName,
			Title:     attendee.Title,
			DealRole:  attendee.DealRole,
			LastTouch: attendee.LastTouch,
			FirstTime: attendee.firstTime(),
		})
		in.LastTouchAt = latest(in.LastTouchAt, attendee.LastTouch)
		in.Commitments = append(in.Commitments, foldClaims(attendee.FullName, perAttendee[attendee.PersonID])...)
	}
	return in
}

// foldClaims turns one attendee's claims into the brief's shape, dropping the
// dismissed ones.
//
// A dismissed claim is one a human said was never true. Writing prep from it
// would resurrect it in front of the person it was wrong about, which is the
// worst place for the correction to have been ignored.
func foldClaims(personName string, found []crmcontracts.ConversationClaim) []ClaimIn {
	out := make([]ClaimIn, 0, len(found))
	for _, claim := range found {
		if claim.Status == crmcontracts.ConversationClaimStatusDismissed {
			continue
		}
		folded := ClaimIn{
			PersonName: personName,
			Kind:       string(claim.Kind),
			Body:       claim.Body,
			Status:     string(claim.Status),
			SourceID:   ids.UUID(claim.SourceActivityId).String(),
			DueAt:      claim.DueAt,
			OccurredAt: claim.OccurredAt,
		}
		if claim.SourceLabel != nil {
			folded.SourceLabel = *claim.SourceLabel
		}
		out = append(out, folded)
	}
	return out
}

// WithCounterpart folds in what the lead attendee's own page already knows:
// where they work, and what was last said.
//
// The 360 is the read the person page itself serves, so anything the brief says
// from it is something the reader could have seen by opening that page. It
// never OVERRIDES the meeting read — the deal on the invite is the deal this
// meeting is about, and the person's leading open deal may be a different one.
func WithCounterpart(in *Input, view crmcontracts.Person360) {
	in.Company = currentEmployer(view)
	if in.Deal == nil {
		in.Deal = dealFromView(view)
	}
	if view.Activities == nil {
		return
	}
	for _, activity := range view.Activities.Data {
		if len(in.Recent) == recentCap {
			break
		}
		// A row outside this caller's audience is kept out of Recent the same
		// way an unreadable row is kept out of History's citable set — it
		// reached this read as a date and a count, on the person's own page,
		// and it must not go on to be the model's or the floor's evidence for
		// a sentence about it. Skipped rather than counted against recentCap,
		// so a withheld row never costs the section a readable one's slot.
		//
		// Spelled as an allow-list, not a deny-list: this feeds the model's
		// prompt, so an unrecognised future content_state must be excluded by
		// default rather than let through because it didn't spell "withheld".
		if activity.ContentState == nil || *activity.ContentState != crmcontracts.ActivityContentStateAvailable {
			continue
		}
		folded := ActIn{
			ID:   ids.UUID(activity.Id).String(),
			Kind: string(activity.Kind),
			At:   activity.OccurredAt.UTC(),
		}
		if activity.Subject != nil {
			folded.Subject = *activity.Subject
		}
		if activity.Direction != nil {
			folded.Direction = string(*activity.Direction)
		}
		in.Recent = append(in.Recent, folded)
	}
}

// recentCap bounds the timeline the deal-state section reads. The brief is a
// two-to-three-minute read; a longer window buys nothing a reader will see.
const recentCap = 10

// dealFromView takes the person's leading open deal when the invite named no
// deal at all. A meeting linked to no deal is common — the calendar event was
// captured before anyone filed it — and refusing to say what is commercially at
// stake because of that would leave the reader worse informed than the person
// page they came from.
func dealFromView(view crmcontracts.Person360) *DealIn {
	if view.Commercial == nil || view.Commercial.Deal == nil {
		return nil
	}
	deal := view.Commercial.Deal
	out := &DealIn{ID: ids.UUID(deal.DealId).String(), Name: deal.Title}
	if deal.Stage != nil {
		out.Stage = *deal.Stage
	}
	if deal.AmountMinor != nil {
		out.AmountMinor = *deal.AmountMinor
	}
	if deal.Currency != nil {
		out.Currency = *deal.Currency
	}
	if deal.CloseDate != nil {
		closeDate := deal.CloseDate.Time
		out.CloseDate = &closeDate
	}
	return out
}

// currentEmployer names where the counterpart works now. The 360 sorts the
// current-primary employment to index zero, so the first row is the answer.
func currentEmployer(view crmcontracts.Person360) string { return personcontext.CurrentEmployer(view) }

// latest keeps the newer of two optional instants, treating nil as "nothing
// captured" rather than as the zero time — the zero time would win every
// comparison and report a relationship as touched in year one.
func latest(current, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	if current == nil || candidate.After(*current) {
		return candidate
	}
	return current
}

func derefInt(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
