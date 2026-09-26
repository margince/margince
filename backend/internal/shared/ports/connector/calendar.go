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
type CalendarAppointment struct {
	Cancel        bool
	EmailReminder bool
	ContactID     ids.UUID
	PassportID    ids.UUID
	CalendarID    string
	EventID       string
	RequestID     string
	Subject       string
	Description   string
	Location      string
	Start         time.Time
	End           time.Time
	Attendees     []string
}

// CalendarReceipt is evidence of a provider event, not guest acceptance.
type CalendarReceipt struct {
	EventID string `json:"event_id"`
	UID     string `json:"uid,omitempty"`
	URL     string `json:"url,omitempty"`
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
