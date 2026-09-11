// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The scheduling verbs, split out of tools_comms.go when that file crossed the
// 500-line cap. The seam they ride (Comms) and its registration still live
// next door, because it serves both families — what separates them is the
// subject: these two answer about TIME and commit a slot, where the mail and
// channel verbs address a person and send them words.
//
// check_availability is 🟢 (it proposes slots and commits nothing);
// book_meeting is 🟡 — it writes a meeting and implies an invitation.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// --- check_availability (🟢 read) ---

type checkAvailability struct{ comms Comms }

func (t checkAvailability) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "check_availability", Title: "Check calendar availability", Version: toolVersionV1,
		Description:   checkAvailabilityCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getAvailability",
		InputSchema: schema(`{"type":"object","required":["from","to"],"properties":{
			"host_user_id":{"type":"string","format":"uuid","description":"Defaults to the acting principal's user"},
			"from":{"type":"string","format":"date-time"` + timestampNote + `},
			"to":{"type":"string","format":"date-time"` + timestampNote + `},
			"duration_minutes":{"type":"integer","minimum":15,"maximum":480}},
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
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	noteDerivedContent(ctx)
	free, err := t.comms.Availability(ctx, args.HostUserID, args.From, args.To, args.DurationMinutes)
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
	case CalendarUnbacked:
		return noCalendarConnectedMessage, true
	case CalendarBackingUnknown:
		return foreignCalendarMessage, true
	default:
		return "", false
	}
}

// noCalendarConnectedMessage is what a reader is told when no calendar backs
// the window. It names the absent conclusion outright — a free slot is not
// evidence that a meeting does not exist — because that is the one a reader
// reached unprompted, in bold, in every run: "There is no meeting with them in
// your calendar tomorrow."
// foreignCalendarMessage is the same caveat for a host who is not the acting
// seat. It withholds the one thing the message above states — whether a
// calendar is connected — because that is the other person's account, and a
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

// --- book_meeting (🟡: commits a slot + implies an invite) ---

// bookMeetingTool carries a record reader for its staging, like the two send
// verbs — but it reads the records the booking will ATTACH to rather than one
// anchor, because a booking has none.
type bookMeetingTool struct {
	comms Comms
	p     datasource.SystemOfRecordProvider
}

func (t bookMeetingTool) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "book_meeting", Title: "Book a meeting", Version: toolVersionV1,
		Description:   bookMeetingCopy.render(),
		RequiredScope: principal.ScopeSend, Tier: mcp.TierAutoExecute, Egress: true,
		OpenAPIOp: "bookMeeting",
		// `links` is REQUIRED by crm.yaml's bookMeeting body and was advertised
		// as optional, so an agent that read the schema and omitted it was
		// refused for a rule the schema never stated. Its vocabulary is spliced
		// from the contract for the same reason log_activity's is: the copy
		// here had drifted to offer a `project` link the contract refuses.
		InputSchema: schema(`{"type":"object","required":["start","end","links"],"properties":{
			"host_user_id":{"type":"string","format":"uuid"},
			"start":{"type":"string","format":"date-time"` + timestampNote + `},
			"end":{"type":"string","format":"date-time"` + timestampNote + `},
			"subject":{"type":"string"},
			"links":{"type":"array","minItems":1,"items":{"type":"object","required":["entity_type","entity_id"],"properties":{
				"entity_type":{"type":"string","enum":` + activityLinkEntityTypeEnum + `},
				"entity_id":{"type":"string","format":"uuid"}},"additionalProperties":false},"maxItems":25,
				"description":"Who and what the meeting is about; at least one. The booking is refused without it."},
			"approval_id":{"type":"string","format":"uuid","description":"Set on approved retry"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[PassthroughEntityResult](),
	}
}

// StageInfo decodes this door's arguments into the booking command and
// delegates: the refusals and the staged subject live in the resolver
// (commandlinked.go), where the REST door reaches the same ones for the same
// operation.
//
// This door's wire shape IS the command's field set — the arguments differ
// only in carrying JSON tags — so it converts rather than restating the
// fields, and a command that grows one fails to compile here instead of
// quietly leaving it unset.
func (t bookMeetingTool) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args BookMeetingArgs
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewBookMeetingCall(t.p, BookMeetingCommand(args)))
}

func (t bookMeetingTool) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args BookMeetingArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	// Both doors, not just staging. This one is reached with an approval already
	// redeemed, so a call that skipped StageInfo would otherwise execute a
	// booking the schema says is impossible — and the cap and the dedupe are
	// part of that: the human approved "attached to N record(s)" as the resolver
	// counted them, and a booking that reaches the seam with the raw list is one
	// whose approval was read against a different reach than the one it takes.
	// The functions are the resolver's own (commandlinked.go), not a second
	// spelling of them.
	if err := requireBookingWindow(args.Start, args.End); err != nil {
		return nil, err
	}
	if err := requireBookingLinks(args.Links); err != nil {
		return nil, err
	}
	// Booking onto SOMEBODY ELSE'S calendar takes admin. This lived only in the
	// resolver's Guards, which run inside StageSubject — enough while the verb
	// always staged, and nothing at all now that it executes: defaultHost
	// (compose/comms.go) takes whatever host it is handed without asking who
	// the caller is.
	if err := requireOwnCalendarOrAdmin(ctx, args.HostUserID); err != nil {
		return nil, err
	}
	links, err := uniqueRecordLinks(args.Links)
	if err != nil {
		return nil, err
	}
	// And every link is read under the caller's row scope, the same reason:
	// a booking must not be filed against a record the caller cannot see.
	if _, err := readStageableLinks(ctx, t.p, links); err != nil {
		return nil, err
	}
	args.Links = links
	for _, link := range links {
		noteEvidence(ctx, datasource.EntityType(link.EntityType), link.EntityID)
	}
	return t.comms.BookMeeting(ctx, args)
}
