// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The activity projection: which columns are selected, and where each one is
// scanned, stated ONCE.
//
// They used to be two lists in two functions that had to track each other by
// eye. Most neighbours differ enough in type that a transposition fails loudly
// on scan — but five in a row here are string-ish and nullable
// (meeting_status, source_system, source_id, source, captured_by), and
// swapping any two of those scans cleanly and puts the wrong value on the
// wire. No error, no failing test, a record that says its source is a meeting
// status.
//
// The order is a single slice now. A column added in the middle of the SELECT
// arrives with its own destination attached, so there is no second list to
// forget.

import (
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// activityScan holds one row mid-flight: the contract record being built, and
// the values that need converting before they can join it.
//
// The temporaries are here rather than local to the scanner because the
// destination of a column is part of that column's declaration below, and a
// closure cannot address a variable that does not exist yet.
type activityScan struct {
	a  crmcontracts.Activity
	id ids.UUID
	// assigneeID and hostUserID are typed ids in the row and openapi UUIDs on
	// the record.
	assigneeID, hostUserID *ids.UUID
	// sourceActivityID is the activity this one was derived from — the meeting
	// whose transcript proposed a task. Scanned as a typed id and mapped to the
	// contract's openapi UUID, like the two above.
	sourceActivityID *ids.UUID
	// language is scanned as text and mapped to the contract enum: the column
	// is a CHECK-constrained string, and a row written before the enum existed
	// must not fail a read.
	language *string
	kind     string
	// The nullable strings that become typed contract enums.
	channelProvider, direction, meetingStatus, threadKey, captureLabel *string
	// audienceReason says why a derived audience is what it is. It travels with
	// the content, not with the markers: "held because personnel" describes
	// what the message is about.
	audienceReason *string
	// Who wrote the row in the system it was imported from, in the two spellings
	// the column pair carries: a seat here (sourceAuthorID, whose current name
	// arrives as sourceAuthorSeatName) or a name the source knew and nothing
	// else (sourceAuthorName). Assembled into the record's one `author` object
	// by authorOf, which is where the precedence between them is decided.
	sourceAuthorID                         *ids.UUID
	sourceAuthorName, sourceAuthorSeatName *string
	// raw is the source system's own representation of an imported activity.
	// It is content — it holds the message text an importer handed over — so
	// it is scanned aside and only reaches the record when the audience test
	// passed, exactly like the subject and body.
	raw              map[string]any
	bulkMailAttested bool
	version          int64
	audience         string
	// contentAvailable is the caller's audience test, evaluated per row: it
	// decides whether the content columns above reach the caller at all.
	contentAvailable bool
}

// activityColumn is one column's whole declaration: how it is SELECTed and
// where it is scanned. Keeping the two halves in one place is the point —
// separately they are two ordered lists, and only one of them fails loudly
// when they disagree.
type activityColumn struct {
	sql  string
	into func(*activityScan) any
}

// activityProjection is the select list every activity read scans, in order.
//
// The last entry is the caller's audience test rendered as content_available —
// the one column that decides whether scanActivity hands back the row's
// content or only its markers. Its SQL is supplied per query, so it carries an
// empty sql here and activityColumns fills it in.
var activityProjection = []activityColumn{
	{"a.id", func(s *activityScan) any { return &s.id }},
	{"a.kind", func(s *activityScan) any { return &s.kind }},
	{"a.channel_provider", func(s *activityScan) any { return &s.channelProvider }},
	{"a.subject", func(s *activityScan) any { return &s.a.Subject }},
	{"a.body", func(s *activityScan) any { return &s.a.Body }},
	{"a.occurred_at", func(s *activityScan) any { return &s.a.OccurredAt }},
	{"a.direction", func(s *activityScan) any { return &s.direction }},
	{"a.due_at", func(s *activityScan) any { return &s.a.DueAt }},
	{"a.remind_at", func(s *activityScan) any { return &s.a.RemindAt }},
	{"a.assignee_id", func(s *activityScan) any { return &s.assigneeID }},
	{"a.is_done", func(s *activityScan) any { return &s.a.IsDone }},
	{"a.done_at", func(s *activityScan) any { return &s.a.DoneAt }},
	{"a.duration_seconds", func(s *activityScan) any { return &s.a.DurationSeconds }},
	{"a.meeting_status", func(s *activityScan) any { return &s.meetingStatus }},
	{"a.host_user_id", func(s *activityScan) any { return &s.hostUserID }},
	{"a.source_system", func(s *activityScan) any { return &s.a.SourceSystem }},
	{"a.source_id", func(s *activityScan) any { return &s.a.SourceId }},
	{"a.source", func(s *activityScan) any { return &s.a.Source }},
	{"a.source_activity_id", func(s *activityScan) any { return &s.sourceActivityID }},
	{"a.language", func(s *activityScan) any { return &s.language }},
	{"a.captured_by", func(s *activityScan) any { return &s.a.CapturedBy }},
	{"a.source_author_id", func(s *activityScan) any { return &s.sourceAuthorID }},
	{"a.source_author_name", func(s *activityScan) any { return &s.sourceAuthorName }},
	// The author's CURRENT display name: sourceAuthorSeatNameSQL, below the table.
	// Three readers compose their own `FROM activity a WHERE …` around this
	// projection, so a join would have to be added to each of them and to the
	// next one somebody writes — which is precisely how the contact360 timeline
	// came to be missing a column for a whole slice, twice. A column that
	// carries its own source needs nothing of its callers.
	//
	// NO liveness filter, deliberately. Who wrote something in August is a fact
	// about August: a colleague who has since left was still its author, and
	// readEmailParties already refuses the same filter for the same reason.
	{sourceAuthorSeatNameSQL, func(s *activityScan) any { return &s.sourceAuthorSeatName }},
	{"a.version", func(s *activityScan) any { return &s.version }},
	{"a.created_at", func(s *activityScan) any { return &s.a.CreatedAt }},
	{"a.updated_at", func(s *activityScan) any { return &s.a.UpdatedAt }},
	{"a.archived_at", func(s *activityScan) any { return &s.a.ArchivedAt }},
	{"a.thread_key", func(s *activityScan) any { return &s.threadKey }},
	{"a.capture_label", func(s *activityScan) any { return &s.captureLabel }},
	{"a.bulk_mail_attested", func(s *activityScan) any { return &s.bulkMailAttested }},
	{"a.audience", func(s *activityScan) any { return &s.audience }},
	{"a.audience_reason", func(s *activityScan) any { return &s.audienceReason }},
	{"a.raw", func(s *activityScan) any { return &s.raw }},
	{"", func(s *activityScan) any { return &s.contentAvailable }},
}

// sourceAuthorSeatNameSQL resolves the author's current display name, and it is
// a SUBSELECT rather than a join.
//
// Three readers compose their own `FROM activity a WHERE …` around this
// projection, so a join would have to be added to each of them and to the next
// one somebody writes — which is precisely how the contact360 timeline came to
// be missing a column for a whole slice, twice. A column that carries its own
// source needs nothing of its callers.
//
// NO liveness filter, deliberately. Who wrote something in August is a fact
// about August: a colleague who has since left was still its author, and
// readEmailParties already refuses the same filter for the same reason.
//
// ALIASED, and the alias is load-bearing even though no activity reader needs
// it today. An unaliased subselect takes its output name from the column inside
// it, so this arrives as `display_name`; a select list that also draws a column
// of that name then has two of them, and `ORDER BY "display_name"` fails with
// SQLSTATE 42702. The record stores' copies of this helper shipped unaliased
// and took the accounts list down for exactly that reason. No activity
// projection draws a `display_name` today — this is aliased so the next one
// does not rediscover it.
const sourceAuthorSeatNameSQL = `(SELECT u.display_name FROM app_user u ` +
	`WHERE u.id = a.source_author_id) AS source_author_seat_name`

// SourceAuthorOf builds the record's `author` from the column pair and the
// seat name the projection resolved, or answers nil when the row has no author.
//
// PRECEDENCE, and it only looks like a detail: the seat's CURRENT name wins
// over the name the source carried. An author who works here may have married,
// corrected a spelling, or been entered into the old system wrong, and the
// directory is the thing that knows. The source's spelling is the fallback for
// somebody who never held a seat — and it is also what survives when a seat is
// hard-deleted, because the FK nulls the id and leaves the name standing.
//
// A row with an id that resolves to nothing keeps the id: the reader still
// learns that an identified member wrote it, and rendering it as unattributed
// would be a stronger claim than the data supports.
func SourceAuthorOf(id *ids.UUID, seatName, sourceName, sourceSystem *string) *crmcontracts.SourceAuthor {
	name := ""
	switch {
	case seatName != nil && *seatName != "":
		name = *seatName
	case sourceName != nil && *sourceName != "":
		name = *sourceName
	}
	if id == nil && name == "" {
		return nil
	}
	return &crmcontracts.SourceAuthor{
		UserId:      uuidPtr(id),
		DisplayName: name,
		Via:         provenance.DisplayVia(sourceSystem),
	}
}

// activityLive is the not-archived predicate, for the alias every read of this
// table uses. One spelling, because three list builders composed it as a
// string literal and a fourth would have been the fourth chance to type it
// differently — a read that filtered on the wrong column would quietly serve
// archived rows rather than fail.
const activityLive = "a.archived_at IS NULL"

// activityColumns renders the select list. contentArm is the predicate
// auth.ActivityAudienceArm rendered for this query's arguments, which is why
// the final column's SQL cannot be a constant.
func activityColumns(contentArm string) string {
	out := make([]string, len(activityProjection))
	for i, c := range activityProjection {
		out[i] = c.sql
	}
	out[len(out)-1] = "(" + contentArm + ") AS content_available"
	return strings.Join(out, ", ")
}

// activityScanTargets answers the destinations, in the projection's order.
func activityScanTargets(s *activityScan) []any {
	dests := make([]any, len(activityProjection))
	for i, c := range activityProjection {
		dests[i] = c.into(s)
	}
	return dests
}

// record finishes the row: the conversions the scan could not do, and the
// content withholding the audience test decided.
func (s *activityScan) record() crmcontracts.Activity {
	a := s.a
	aud := crmcontracts.ActivityAudience(s.audience)
	a.Audience = &aud
	threadKey, captureLabel, audienceReason := s.threadKey, s.captureLabel, s.audienceReason
	state := crmcontracts.ActivityContentStateAvailable
	if !s.contentAvailable {
		// Withheld: the row is discoverable, its content is not the caller's.
		// Everything that carries what was said — or identifies the message
		// at the provider — goes; the markers stay.
		state = crmcontracts.ActivityContentStateWithheld
		a.Subject, a.Body, a.SourceId = nil, nil, nil
		// The language too: it was read off the body, so answering it would
		// tell a caller one fact about text they may not read.
		a.Language = nil
		threadKey, captureLabel, audienceReason = nil, nil, nil
	}
	a.ContentState = &state

	a.Id = openapi_types.UUID(s.id)
	a.AssigneeId = uuidPtr(s.assigneeID)
	// The meeting a task was read out of. A marker, not content: which record
	// produced this one is a fact like its date, and opening it is a separate
	// read under the caller's own scope.
	a.SourceActivityId = uuidPtr(s.sourceActivityID)
	// Our own side of a meeting. Not gated by the content audience: who held a
	// meeting is a marker like its date and its direction, and a caller who may
	// discover the row may know whose meeting it was.
	a.HostUserId = uuidPtr(s.hostUserID)
	// Who wrote it in the system it came from — WITHHELD with the content, not
	// carried like the host above.
	//
	// The host is a colleague's seat on our own side of a meeting. This is a
	// free-text name that arrived with imported text, about a human who is
	// usually neither party to the exchange, and the Art. 17 redaction clears
	// it alongside the subject and the body for exactly that reason. A field
	// the erasure treats as content cannot be a marker here: a reader outside a
	// held message's audience would learn who wrote it while being refused
	// every word of it.
	if s.contentAvailable {
		a.Author = SourceAuthorOf(s.sourceAuthorID, s.sourceAuthorSeatName, s.sourceAuthorName, s.a.SourceSystem)
	}
	if s.language != nil && s.contentAvailable {
		lang := crmcontracts.ActivityLanguage(*s.language)
		a.Language = &lang
	}
	// raw carries the importer's own copy of the message, so it reaches the
	// caller only when the body does: serving it to a withheld reader would
	// hand back in another shape exactly what was just withheld.
	if s.contentAvailable && s.raw != nil {
		raw := s.raw
		a.Raw = &raw
	}
	a.Kind = crmcontracts.ActivityKind(s.kind)
	a.ChannelProvider = s.channelProvider
	if s.direction != nil {
		d := crmcontracts.ActivityDirection(*s.direction)
		a.Direction = &d
	}
	if s.meetingStatus != nil {
		m := crmcontracts.ActivityMeetingStatus(*s.meetingStatus)
		a.MeetingStatus = &m
	}
	a.ThreadKey = threadKey
	a.AudienceReason = audienceReason
	if captureLabel != nil {
		label := crmcontracts.ActivityCaptureLabel(*captureLabel)
		a.CaptureLabel = &label
	}
	bulk := s.bulkMailAttested
	a.BulkMailAttested = &bulk
	version := s.version
	a.Version = &version
	// What a canonical email row draws, composed here so every reader of an
	// activity carries it: a list that had to fetch a message per visible line
	// to draw one would be the N+1 this field exists to avoid. Attached from
	// the row's own columns only — the counterparty and the attachment count
	// need joins this scan does not have, and the detail read fills them in.
	a.EmailSummary = RowEmailSummary(a)
	return a
}

// RowEmailSummary is the email row's fields, for the kind that has them.
//
// Exported because compose/contact360 assembles its own Activity from a
// hand-written twin of this projection and cannot reach record(). That twin is
// why this is exported rather than private: it has gone missing a column twice
// before, and a summary it did not carry would make the contract's "present
// exactly when kind=email" false on the contact page alone.
//
// Present exactly when kind=email, so a reader branches on the field rather
// than on the kind word: a call and a note are activities too, and neither has
// an email's shape. A withheld row still gets a summary — the status says the
// content is not this caller's, which is what keeps a withheld row visibly
// withheld rather than absent.
func RowEmailSummary(a crmcontracts.Activity) *crmcontracts.EmailSummary {
	if a.Kind != crmcontracts.ActivityKindEmail {
		return nil
	}
	withheld := a.ContentState != nil && *a.ContentState == crmcontracts.ActivityContentStateWithheld
	summary := crmcontracts.EmailSummary{
		ActivityId:    a.Id,
		OccurredAt:    a.OccurredAt,
		DisplayStatus: crmcontracts.EmailAccessStatusWithheld,
		Move:          crmcontracts.EmailSummaryMoveNone,
	}
	if a.Version != nil {
		summary.Version = *a.Version
	}
	if a.Direction != nil {
		d := crmcontracts.EmailSummaryDirection(*a.Direction)
		summary.Direction = &d
	}
	if withheld {
		return &summary
	}
	// The narrow word, always. This projection has no transaction and so cannot
	// ask whether a workspace audience is narrowed by the records the message
	// is filed against; WithEmailRowFacts asks that for the whole page and
	// widens the rows that reach everyone. A label that guesses guesses DOWN.
	summary.DisplayStatus = crmcontracts.EmailAccessStatusTeam
	if a.Audience != nil {
		summary.DisplayStatus = narrowStatusForAudience(*a.Audience)
	}
	summary.Subject = a.Subject
	if a.Body != nil {
		if preview := EmailSummaryText(*a.Body); preview != "" {
			summary.Preview = &preview
		}
	}
	return &summary
}
