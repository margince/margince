// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Names come from the owner-scoped reader. Missing names stay generic so a
// receipt cannot reveal a contact that reader was not allowed to see.

import (
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pacedBasis names the observations behind an unconfirmed estimate.
func (said quietCopy) pacedBasis(remainingStages int, pace closeDatePace) string {
	return fmt.Sprintf(said.Say(quietWords.Paced), pace.wonDeals, pace.days, remainingStages)
}

// QuietNames maps the counterparty on each side of ReadQuietFacts to a display
// name. A side whose contact is unknown — an unmatched address, an erased link,
// or a reader without contact:read — is simply absent from the map.
type QuietNames map[ids.UUID]string

// quietReason composes the sentence. `today` is the sweep's own day so the
// day counts land in the workspace's calendar rather than the server's.
func (said quietCopy) quietReason(facts QuietFacts, names QuietNames, today time.Time, loc *time.Location) string {
	inbound, outbound := facts.LastInbound, facts.LastOutbound
	if inbound != nil && (outbound == nil || outbound.At.Before(inbound.At)) {
		phrase := quietWords.InMessage
		switch crmcontracts.ActivityKind(inbound.Kind) {
		case crmcontracts.ActivityKindCall:
			phrase = quietWords.InCall
		case crmcontracts.ActivityKindMeeting:
			phrase = quietWords.InMeeting
		}
		return fmt.Sprintf(said.Say(phrase), quietWho(inbound, names, said.Say(quietWords.Contact)), quietDay(inbound.At, loc), said.quietFor(inbound.At, today, loc))
	}
	if outbound != nil {
		phrase, unknown := quietWords.OutMessage, quietWords.ContactRecipient
		switch crmcontracts.ActivityKind(outbound.Kind) {
		case crmcontracts.ActivityKindCall:
			phrase, unknown = quietWords.OutCall, quietWords.ContactObject
		case crmcontracts.ActivityKindMeeting:
			phrase, unknown = quietWords.OutMeeting, quietWords.ContactObject
		}
		return fmt.Sprintf(said.Say(phrase), quietWho(outbound, names, said.Say(unknown)), quietDay(outbound.At, loc), said.quietFor(outbound.At, today, loc))
	}
	return said.Say(quietWords.NoCorrespondence)
}

// quietWho names the counterparty, falling back to a generic noun. The fallback
// differs by position in the sentence, so the caller supplies it: "The contact
// wrote" opens a sentence, "we wrote to the contact" does not.
func quietWho(side *QuietSide, names QuietNames, unknown string) string {
	if name, ok := names[side.ContactID]; ok && name != "" {
		return name
	}
	return unknown
}

// quietDay is the date a reader recognises, not a wire timestamp — and it is
// the date in the WORKSPACE's zone, the same zone the sweep decides which day a
// deal is late on. A message at 23:00 local is otherwise reported as the next
// day, which a reader can check against their own mailbox and find wrong.
func quietDay(at time.Time, loc *time.Location) string {
	return at.In(orUTC(loc)).Format(time.DateOnly)
}

// orUTC is the zone fallback. A sweep always has one; the guard exists so a
// caller that does not cannot panic a nil dereference into the nightly pass.
func orUTC(loc *time.Location) *time.Location {
	if loc == nil {
		return time.UTC
	}
	return loc
}

// quietFor is the elapsed span, in the unit a contact would say it in. Days up to
// a fortnight, then weeks: "38 days" makes a reader do arithmetic to learn what
// "5 weeks" says outright.
func (said quietCopy) quietFor(since, today time.Time, loc *time.Location) string {
	zone := orUTC(loc)
	days := max(elapsed.Days(since, today, zone), 0)
	switch {
	case days == 0:
		return said.Say(quietWords.Today)
	case days == 1:
		return said.Say(quietWords.Yesterday)
	case days <= 14:
		return fmt.Sprintf(said.Say(quietWords.Days), days)
	default:
		return fmt.Sprintf(said.Say(quietWords.Weeks), days/7)
	}
}
