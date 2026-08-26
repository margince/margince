// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The sentence a person reads when asked whether a quiet deal is still alive.
//
// It replaced a fixed string ("deal has gone quiet; confirm it is still alive")
// which was true of every quiet deal and therefore told the reader nothing they
// could act on. A reason earns its place by naming what happened: which way the
// silence runs, who is on the other end of it, and how long it has lasted.
//
// The three shapes below are not stylistic variants — they are three different
// situations, and the verb changes because the next action does:
//
//	they wrote last  → we owe a reply, and the name is who we owe it to
//	we wrote last    → they went cold, and the name is who stopped answering
//	neither          → nobody has ever corresponded on this deal at all
//
// Names arrive already resolved. This module cannot read the person table, and
// a reason that silently drops the name when the reader lacks person:read is
// better than one that leaks it — so an unknown name degrades to "the contact"
// rather than to an identifier or an empty gap.

import (
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The reasons that are not about a deal's correspondence, kept beside the ones
// that are so all of this surface's prose reads as one voice.
const (
	// quietFallbackBasis stands in when the correspondence could not be read.
	// It says the deal is quiet and does not pretend to know more, which is
	// the honest version of what the sweep has at that point.
	quietFallbackBasis = "This deal has gone quiet and we could not read its correspondence — check the account before confirming."

	// quietHoldingBasis explains a card that is simply still waiting: the
	// sweep already set this date, and nobody has confirmed it yet.
	quietHoldingBasis = "This date was set automatically by an earlier nightly check and has not been confirmed by anyone yet."
)

// pacedBasis says where a proposed date came from, without showing the
// arithmetic. "3 open stage(s) remaining × stage velocity" is the formula that
// produced the number, and a formula is not a reason a salesperson can weigh.
func pacedBasis(remainingStages int) string {
	if remainingStages == 1 {
		return "Based on how long deals like this usually take to clear their last stage."
	}
	return fmt.Sprintf(
		"Based on how long deals like this usually take, with %d stages still to go.",
		remainingStages)
}

// QuietNames maps the counterparty on each side of ReadQuietFacts to a display
// name. A side whose person is unknown — an unmatched address, an erased link,
// or a reader without person:read — is simply absent from the map.
type QuietNames map[ids.UUID]string

// quietReason composes the sentence. `today` is the sweep's own day so the
// day counts land in the workspace's calendar rather than the server's.
func quietReason(facts QuietFacts, names QuietNames, today time.Time, loc *time.Location) string {
	inbound, outbound := facts.LastInbound, facts.LastOutbound
	switch {
	case inbound != nil && (outbound == nil || outbound.At.Before(inbound.At)):
		// They spoke last: the ball is ours, and that is the sharper finding of
		// the two because it is a thing WE failed to do.
		return fmt.Sprintf("%s %s on %s and %s — %s.",
			quietWho(inbound, names, "The contact"),
			quietReached(inbound.Kind),
			quietDay(inbound.At, loc),
			quietUnanswered(inbound.Kind),
			quietFor(inbound.At, today, loc))
	case outbound != nil:
		return fmt.Sprintf("We %s %s on %s and %s — %s.",
			quietReachedOut(outbound.Kind),
			quietWho(outbound, names, "the contact"),
			quietDay(outbound.At, loc),
			quietSilenceSince(outbound.Kind),
			quietFor(outbound.At, today, loc))
	default:
		return "Nobody has been in touch on this deal either way — there is no correspondence to judge it by."
	}
}

// quietReached and quietReachedOut say what actually happened, per direction.
// The facts query accepts any directional activity, so a phone call would
// otherwise be reported as something somebody "wrote" — a small falsehood, and
// a reader who was actually on that call stops trusting the rest of the
// sentence. An unrecognised kind degrades to the neutral verb rather than
// guessing.
func quietReached(kind string) string {
	switch crmcontracts.ActivityKind(kind) {
	case crmcontracts.ActivityKindCall:
		return "called"
	case crmcontracts.ActivityKindMeeting:
		return "met us"
	default:
		return "wrote"
	}
}

func quietReachedOut(kind string) string {
	switch crmcontracts.ActivityKind(kind) {
	case crmcontracts.ActivityKindCall:
		return "called"
	case crmcontracts.ActivityKindMeeting:
		return "met"
	default:
		return "wrote to"
	}
}

// quietUnanswered and quietSilenceSince close the sentence in the same terms it
// opened. "Called ... and nobody has answered" reads as a missed phone call
// rather than an unreturned one, and a meeting never gets a "reply" at all — so
// the second half of the sentence follows the verb in the first.
func quietUnanswered(kind string) string {
	switch crmcontracts.ActivityKind(kind) {
	case crmcontracts.ActivityKindCall, crmcontracts.ActivityKindMeeting:
		return "nobody has followed up since"
	default:
		return "nobody has answered since"
	}
}

func quietSilenceSince(kind string) string {
	switch crmcontracts.ActivityKind(kind) {
	case crmcontracts.ActivityKindCall, crmcontracts.ActivityKindMeeting:
		return "nothing has happened since"
	default:
		return "there has been no reply"
	}
}

// quietWho names the counterparty, falling back to a generic noun. The fallback
// differs by position in the sentence, so the caller supplies it: "The contact
// wrote" opens a sentence, "we wrote to the contact" does not.
func quietWho(side *QuietSide, names QuietNames, unknown string) string {
	if name, ok := names[side.PersonID]; ok && name != "" {
		return name
	}
	return unknown
}

// quietDay is the date a reader recognises, not a wire timestamp — and it is
// the date in the WORKSPACE's zone, the same zone the sweep decides which day a
// deal is late on. A message at 23:00 local is otherwise reported as the next
// day, which a reader can check against their own mailbox and find wrong.
func quietDay(at time.Time, loc *time.Location) string {
	return at.In(orUTC(loc)).Format("2 January")
}

// orUTC is the zone fallback. A sweep always has one; the guard exists so a
// caller that does not cannot panic a nil dereference into the nightly pass.
func orUTC(loc *time.Location) *time.Location {
	if loc == nil {
		return time.UTC
	}
	return loc
}

// dayIn reduces an instant to its calendar day in the given zone, so the span
// counts the days a reader would count rather than 24-hour blocks.
func dayIn(at time.Time, loc *time.Location) time.Time {
	y, m, d := at.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// quietFor is the elapsed span, in the unit a person would say it in. Days up to
// a fortnight, then weeks: "38 days" makes a reader do arithmetic to learn what
// "5 weeks" says outright.
func quietFor(since, today time.Time, loc *time.Location) string {
	zone := orUTC(loc)
	days := int(dayIn(today, zone).Sub(dayIn(since, zone)).Hours() / 24)
	if days < 0 {
		days = 0
	}
	switch {
	case days == 0:
		return "that was today"
	case days == 1:
		return "that was yesterday"
	case days <= 14:
		return fmt.Sprintf("%d days ago", days)
	default:
		return fmt.Sprintf("%d weeks ago", days/7)
	}
}
