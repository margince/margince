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
	kind                   string
	// The nullable strings that become typed contract enums.
	channelProvider, direction, meetingStatus, threadKey, captureLabel *string
	// audienceReason says why a derived audience is what it is. It travels with
	// the content, not with the markers: "held because personnel" describes
	// what the message is about.
	audienceReason   *string
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
	{"a.captured_by", func(s *activityScan) any { return &s.a.CapturedBy }},
	{"a.version", func(s *activityScan) any { return &s.version }},
	{"a.created_at", func(s *activityScan) any { return &s.a.CreatedAt }},
	{"a.updated_at", func(s *activityScan) any { return &s.a.UpdatedAt }},
	{"a.archived_at", func(s *activityScan) any { return &s.a.ArchivedAt }},
	{"a.thread_key", func(s *activityScan) any { return &s.threadKey }},
	{"a.capture_label", func(s *activityScan) any { return &s.captureLabel }},
	{"a.bulk_mail_attested", func(s *activityScan) any { return &s.bulkMailAttested }},
	{"a.audience", func(s *activityScan) any { return &s.audience }},
	{"a.audience_reason", func(s *activityScan) any { return &s.audienceReason }},
	{"", func(s *activityScan) any { return &s.contentAvailable }},
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
		threadKey, captureLabel, audienceReason = nil, nil, nil
	}
	a.ContentState = &state

	a.Id = openapi_types.UUID(s.id)
	a.AssigneeId = uuidPtr(s.assigneeID)
	// Our own side of a meeting. Not gated by the content audience: who held a
	// meeting is a marker like its date and its direction, and a caller who may
	// discover the row may know whose meeting it was.
	a.HostUserId = uuidPtr(s.hostUserID)
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
// Exported because compose/person360 assembles its own Activity from a
// hand-written twin of this projection and cannot reach record(). That twin is
// why this is exported rather than private: it has gone missing a column twice
// before, and a summary it did not carry would make the contract's "present
// exactly when kind=email" false on the person page alone.
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
	summary.DisplayStatus = crmcontracts.EmailAccessStatusTeam
	if a.Audience != nil {
		summary.DisplayStatus = statusForAudience(*a.Audience)
	}
	summary.Subject = a.Subject
	if a.Body != nil {
		if preview := EmailSummaryText(*a.Body); preview != "" {
			summary.Preview = &preview
		}
	}
	summary.Move = moveOf(a)
	return &summary
}
