// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A calendar meeting landed and called off through the real Sink, the rows a
// live calendar pull produces, for any suite that must not hand-insert them.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// CalendarSystem is the provider every CalendarMeeting is captured from.
const CalendarSystem = "gcal"

// CalendarSink is a bare Sink plus the one seam the cancel path needs: the
// meeting closer, wired to the module that owns the activity table. No other
// option compose sets on a calendar Sink is wired here.
func (e *Env) CalendarSink() *capture.Sink {
	return capture.NewSink(e.DB()).WithMeetingCloser(
		activities.CancelCapturedMeetingFor(capture.SeatHoldsActivityTx),
		activities.CancelMeetingByIDTx)
}

// ConnectorOwnerCtx binds the connector principal a sync runs under, the way
// capture.Registry builds it: the acting connector is the named one, and
// UserID is the seat whose calendar or mailbox it is.
func (e *Env) ConnectorOwnerCtx(owner ids.UUID, connectorName string) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:" + connectorName,
		UserID: owner, OnBehalfOf: owner,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				objActivity: {Create: true, Read: true, Update: true},
				objContact:  {Create: true, Read: true, Update: true},
				objCompany:  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// CalendarMeeting is one meeting on a seat's calendar: Seat's, or the admin's
// when Seat is zero. That seat hosts it. It has no direction, because a
// calendar capture never records one. A stated Identity binds it to the row
// that already holds that occurrence, as a live calendar's sink does.
type CalendarMeeting struct {
	Event    string
	At       time.Time
	Links    []datasource.EntityRef
	Parties  []connector.MessageParticipant
	Seat     ids.UUID
	Identity connector.CrossDoorIdentity
	// DeliveredTo is the calendar owner's address. It records the seat's
	// import of the meeting where the seat's connection proves that address.
	DeliveredTo string
}

// Capture lands the meeting as a calendar pull does and answers its id.
func (m CalendarMeeting) Capture(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	return m.CaptureThrough(t, e, e.CalendarSink())
}

// CaptureThrough is Capture through sink, with the cross-door identity wired
// on it as compose wires a live calendar's.
func (m CalendarMeeting) CaptureThrough(t *testing.T, e *Env, sink *capture.Sink) ids.UUID {
	t.Helper()
	seat := m.Seat
	if seat.IsZero() {
		seat = e.AdminUser
	}
	sink = sink.WithMessageIdentity(
		activities.IdentityKindMail, activities.IdentityKindMeeting, activities.MeetingIdentityKey,
		activities.ResolveBindableIdentity, activities.ClaimIdentity)
	ref, err := sink.Upsert(e.ConnectorOwnerCtx(seat, CalendarSystem), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: CalendarSystem, SourceID: m.Event},
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Account review", OccurredAt: m.At,
		},
		Links:             m.Links,
		Participants:      m.Parties,
		Source:            CalendarSystem + ":" + m.Event,
		CapturedBy:        "connector:" + CalendarSystem,
		Raw:               []byte(`{"id":"` + m.Event + `"}`),
		CrossDoorIdentity: m.Identity,
		DeliveredTo:       m.DeliveredTo,
	}.WithProviderAttestedParticipants(true))
	if err != nil {
		t.Fatalf("capturing meeting %s: %v", m.Event, err)
	}
	return ref.ID
}

// Cancel calls the meeting off as the pull carrying its cancellation does.
func (m CalendarMeeting) Cancel(t *testing.T, e *Env) {
	t.Helper()
	key := connector.NaturalKey{SourceSystem: CalendarSystem, SourceID: m.Event}
	if err := e.CalendarSink().CancelMeeting(e.ConnectorOwnerCtx(e.AdminUser, CalendarSystem), key, m.At); err != nil {
		t.Fatalf("canceling meeting %s: %v", m.Event, err)
	}
}

// RecordHeld records a captured meeting's outcome as held, the way a member
// reports it through the activities writer.
func (e *Env) RecordHeld(t *testing.T, meeting ids.UUID) {
	t.Helper()
	held := "held"
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](meeting),
		activities.UpdateActivityInput{MeetingStatus: &held}); err != nil {
		t.Fatalf("recording meeting %s held: %v", meeting, err)
	}
}
