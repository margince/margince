// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CalendarScheduler is separate from capture: private busy time never becomes
// CRM content, and invitation writes require an explicit provider grant.
type CalendarScheduler interface {
	CalendarList(context.Context, Auth) ([]CalendarOption, error)
	CalendarBusy(context.Context, Auth, string, time.Time, time.Time) ([]CalendarInterval, error)
	CalendarInspect(context.Context, Auth, string, string) (CalendarState, error)
	CalendarLookup(context.Context, Auth, CalendarAppointment) (*CalendarReceipt, error)
	CalendarSave(context.Context, Auth, CalendarAppointment) (CalendarReceipt, error)
	CalendarCancel(context.Context, Auth, string, string) error
}

// CalendarInterval exposes occupancy without event titles or attendee identities.
type CalendarInterval struct {
	EventID string    `json:"-"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
}

// CalendarAppointment carries the stable intent and authority for provider delivery.
// The tags SPELL THE KEYS ALREADY STORED, which are Go's default field names: this
// value goes to jsonb through pgx and `meeting_invitation.appointment` is full of
// rows written that way. Tagging in the ordinary lower_snake style would orphan every
// one of them, since SQL predicates read the key and not the field.
//
// Explicit is the point rather than the spelling. Untagged, the wire format was
// whatever the field was called, so a rename silently changed what the predicates in
// activities/scheduling_*.go and privacy/erasure_payloads.go look for — no compiler
// error, no failing test, the query simply stops matching. With the tags the name and
// the key are separate decisions, and renaming the field changes nothing on the wire.
//
//nolint:tagliatelle // the keys are already in meeting_invitation.appointment, written before anything tagged this; house style here would orphan every stored row
type CalendarAppointment struct {
	Cancel        bool      `json:"Cancel"`
	EmailReminder bool      `json:"EmailReminder"`
	ContactID     ids.UUID  `json:"ContactID"`
	PassportID    ids.UUID  `json:"PassportID"`
	CalendarID    string    `json:"CalendarID"`
	EventID       string    `json:"EventID"`
	RequestID     string    `json:"RequestID"`
	Subject       string    `json:"Subject"`
	Description   string    `json:"Description"`
	Location      string    `json:"Location"`
	Start         time.Time `json:"Start"`
	End           time.Time `json:"End"`
	Attendees     []string  `json:"Attendees"`
	// VideoCall asks the provider for a conference on create only; an update
	// keeps whatever conference the event already has.
	VideoCall bool `json:"VideoCall"`
}

// CalendarReceipt is evidence of a provider event, not guest acceptance.
type CalendarReceipt struct {
	EventID string `json:"event_id"`
	UID     string `json:"uid,omitempty"`
	URL     string `json:"url,omitempty"`
	// VideoURL stays empty when the provider could not create a conference;
	// the event itself was still delivered.
	VideoURL string `json:"video_url,omitempty"`
}

// CalendarOption names a selectable provider calendar and its current write authority.
type CalendarOption struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Writable bool   `json:"writable"`
	Primary  bool   `json:"primary"`
}

// CalendarState reports the provider’s current event interval or cancellation.
type CalendarState struct {
	Start    time.Time
	End      time.Time
	Canceled bool
}
