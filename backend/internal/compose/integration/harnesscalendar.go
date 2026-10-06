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

// CalendarSink is the Sink as compose builds it for a calendar connector: the
// cancel seam wired to the module that owns the activity table.
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
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// CalendarMeeting is one meeting on the admin seat's calendar.
type CalendarMeeting struct {
	Event     string
	At        time.Time
	Direction string
	Links     []datasource.EntityRef
	Parties   []connector.MessageParticipant
}

// Capture lands the meeting as a calendar pull does and answers its id.
func (m CalendarMeeting) Capture(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	ref, err := e.CalendarSink().Upsert(e.ConnectorOwnerCtx(e.AdminUser, CalendarSystem), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: CalendarSystem, SourceID: m.Event},
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Account review", OccurredAt: m.At, Direction: m.Direction,
		},
		Links:        m.Links,
		Participants: m.Parties,
		Source:       CalendarSystem + ":" + m.Event,
		CapturedBy:   "connector:" + CalendarSystem,
		Raw:          []byte(`{"id":"` + m.Event + `"}`),
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
