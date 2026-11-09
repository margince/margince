// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"context"
	"net/http"
	"net/url"

	"github.com/margince/margince/backend/internal/modules/capture/calendarwire"
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type responderAPI interface {
	Respond(ctx context.Context, token, uid, response string) error
}

// CalendarRespond answers an invitation on the owner's primary calendar.
func (c *Connector) CalendarRespond(ctx context.Context, auth connector.Auth, uid, response string) error {
	_, token, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return err
	}
	api, ok := c.api.(responderAPI)
	if !ok {
		return connector.ErrUnreachable
	}
	return api.Respond(ctx, token, uid, response)
}

// invitedEvent keeps every attendee as Google sent it: a patch replaces the
// whole list, so a field this code does not name must still go back.
type invitedEvent struct {
	ID        string           `json:"id"`
	Attendees []map[string]any `json:"attendees"`
}

// Respond finds the event by its iCalendar UID and sets the owner's own
// responseStatus. Google spells the three answers as the contract does.
func (a *httpAPI) Respond(ctx context.Context, token, uid, response string) error {
	var found struct {
		Items []invitedEvent `json:"items"`
	}
	query := url.Values{"iCalUID": {uid}, "maxResults": {"1"}}
	events := a.base + "/calendars/primary/events"
	if _, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, events+"?"+query.Encode(), nil, &found); err != nil {
		return err
	}
	if len(found.Items) == 0 || !answerAsSelf(found.Items[0].Attendees, response) {
		return connector.ErrCalendarEventMissing
	}
	event := found.Items[0]
	patch := map[string]any{"attendees": event.Attendees}
	_, err := calendarwire.Request(ctx, a.client, token, http.MethodPatch,
		events+"/"+url.PathEscape(event.ID)+"?sendUpdates=all", patch, nil)
	return err
}

// answerAsSelf sets the response of the attendee Google marks as the
// calendar's owner, and reports whether there was one.
func answerAsSelf(attendees []map[string]any, response string) bool {
	for _, attendee := range attendees {
		if self, ok := attendee["self"].(bool); ok && self {
			attendee["responseStatus"] = response
			return true
		}
	}
	return false
}

var _ connector.CalendarResponder = (*Connector)(nil)
