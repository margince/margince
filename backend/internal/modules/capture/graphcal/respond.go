// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture/calendarwire"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type responderAPI interface {
	Respond(ctx context.Context, token, uid, response string) error
}

// CalendarRespond answers an invitation on the owner's own calendar.
func (c *Connector) CalendarRespond(ctx context.Context, auth connector.Auth, uid, response string) error {
	token, err := c.calendarAccess(ctx, auth)
	if err != nil {
		return err
	}
	api, ok := c.api.(responderAPI)
	if !ok {
		return connector.ErrUnreachable
	}
	return api.Respond(ctx, token, uid, response)
}

// graphAnswers are Microsoft's action names for the contract's answers.
var graphAnswers = map[string]string{
	"accepted":  "accept",
	"tentative": "tentativelyAccept",
	"declined":  "decline",
}

// Respond finds the event by its iCalendar UID and posts the answer action,
// which sends the organizer a response.
func (a *httpAPI) Respond(ctx context.Context, token, uid, response string) error {
	action, ok := graphAnswers[response]
	if !ok {
		return connector.ErrCalendarEventMissing
	}
	// OData quotes a literal with single quotes and doubles one inside it.
	filter := "iCalUId eq '" + strings.ReplaceAll(uid, "'", "''") + "'"
	query := url.Values{"$filter": {filter}, "$select": {"id"}, "$top": {"1"}}
	var found struct {
		Events []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if _, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, a.base+"/me/events?"+query.Encode(), nil, &found); err != nil {
		return err
	}
	if len(found.Events) == 0 {
		return connector.ErrCalendarEventMissing
	}
	_, err := calendarwire.Request(ctx, a.client, token, http.MethodPost,
		a.base+"/me/events/"+url.PathEscape(found.Events[0].ID)+"/"+action,
		map[string]bool{"sendResponse": true}, nil)
	return err
}

var _ connector.CalendarResponder = (*Connector)(nil)
