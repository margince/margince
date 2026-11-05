// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The scheduling verbs, split out of tools_comms.go when that file crossed the
// 500-line cap. The seam they ride (Comms) and its registration still live
// next door, because it serves both families — what separates them is the
// subject: these two answer about TIME and commit a slot, where the mail and
// channel verbs address a contact and send them words.
//
// check_availability is 🟢 (it proposes slots and commits nothing);
// invite_meeting is the one booking verb: it delivers a real calendar invitation.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// --- check_availability (🟢 read) ---

type checkAvailability struct{ comms Comms }

func (t checkAvailability) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "check_availability", Title: "Check calendar availability", Version: toolVersionV1,
		Description:   checkAvailabilityCopy.render(),
		Instead:       checkAvailabilityCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getAvailability",
		InputSchema: schema(`{"type":"object","required":["from","to"],"properties":{
			"host_user_id":{"type":"string","format":"uuid","description":"Defaults to the acting principal's user"},
			"from":{"type":"string","format":"date-time"` + windowNote + `},
			"to":{"type":"string","format":"date-time"` + windowNote + `},
			"duration_minutes":{"type":"integer","minimum":15,"maximum":480},"reliable":{"type":"boolean","default":false,"description":"Require live calendar occupancy and booking policy; fails if unavailable"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[AvailabilityResult](),
	}
}

func (t checkAvailability) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		HostUserID      *ids.UUID `json:"host_user_id"`
		From            time.Time `json:"from"`
		To              time.Time `json:"to"`
		DurationMinutes int       `json:"duration_minutes"`
		Reliable        bool      `json:"reliable"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	noteDerivedContent(ctx)
	var free AvailabilityResult
	var err error
	if args.Reliable {
		scheduling, ok := t.comms.(ReliableScheduling)
		if !ok {
			return nil, errors.New("calendar-backed availability is unavailable")
		}
		free, err = scheduling.ReliableAvailability(ctx, args.HostUserID, args.From, args.To, args.DurationMinutes)
	} else {
		free, err = t.comms.Availability(ctx, args.HostUserID, args.From, args.To, args.DurationMinutes)
	}
	if err != nil {
		return nil, err
	}
	if message, owed := calendarCaveat(free.CalendarBacking); owed {
		// Both halves, because each is wrong without the other. The warning is
		// the only thing that reaches a model reading prose; dropping the
		// authority claim is the only thing that reaches a client branching on
		// the envelope, and an answer that keeps it is claiming to be the last
		// word on a diary this product was never shown.
		noteWarning(ctx, warningNoCalendarConnected, message)
		noteAnswerLacksItsSource(ctx)
	}
	return marshalResult(free, nil)
}

// calendarCaveat is what the reader is owed about where this window came from,
// and whether anything is owed at all.
//
// The unknown case says the SAME thing as the unbacked one about the answer —
// it is CRM-derived and proves nothing about a diary — and deliberately says
// nothing about the host's account, because that host is not the acting seat
// and their connector state is not this tool's to report.
func calendarCaveat(backing CalendarBacking) (string, bool) {
	switch backing {
	case CalendarBacked:
		// The ONLY value that buys silence, named rather than defaulted to.
		// CalendarBacking's zero value is the empty string, so a result built
		// without setting it used to fall through a `default: no caveat` and
		// keep freshness.authoritative — an answer claiming to be the last word
		// on a diary nobody established it had read. That is this file's own
		// defect reached by forgetting a field, and a value added later would
		// have inherited it.
		return "", false
	case CalendarUnbacked:
		return noCalendarConnectedMessage, true
	default:
		// Unknown, unset, or a state added after this was written. All three
		// mean the same thing to a reader: nothing here establishes the host's
		// diary, and nothing may be said about their account.
		return foreignCalendarMessage, true
	}
}

// noCalendarConnectedMessage is what a reader is told when no calendar backs
// the window. It names the absent conclusion outright — a free slot is not
// evidence that a meeting does not exist — because that is the one a reader
// reached unprompted, in bold, in every run: "There is no meeting with them in
// your calendar tomorrow."
// foreignCalendarMessage is the same caveat for a host who is not the acting
// seat. It withholds the one thing the message above states — whether a
// calendar is connected — because that is the other contact's account, and a
// caller able to ask it for any user id could read the whole roster's connector
// state and watch a grant fail.
const foreignCalendarMessage = "This window is what the meetings recorded in this CRM leave open for that " +
	"host — NOT what their own diary leaves open, and this seat cannot tell whether one is connected to " +
	"it. A free slot here means nothing was recorded, never that they are free, and this answer is no " +
	"evidence that a meeting is missing from their calendar. Offer a time rather than reporting them free."

const noCalendarConnectedMessage = "No calendar is connected for this host, so these slots are what the " +
	"meetings recorded in this CRM leave open — NOT what their diary leaves open. A free slot here means " +
	"nothing was recorded, never that the host is free, and this answer is no evidence that a meeting the " +
	"user told you about is missing from their calendar. Say the calendar is not connected rather than " +
	"reporting the day as clear."

// ReliableScheduling distinguishes live calendar occupancy from CRM-only availability.
type ReliableScheduling interface {
	ReliableAvailability(context.Context, *ids.UUID, time.Time, time.Time, int) (AvailabilityResult, error)
}
