// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//nolint:tagliatelle // Calendar provider field names are an external wire contract.
package graphcal

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/margince/margince/backend/internal/modules/capture/calendarwire"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const meetingRequestProperty = "String {bf79a9ce-556f-4d93-a07d-c67982e87490} Name MarginceMeeting"

type requestProperty struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

func (a *httpAPI) Lookup(ctx context.Context, token string, in connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	var event scheduledEvent
	if in.EventID != "" {
		if _, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, a.base+"/me/events/"+url.PathEscape(in.EventID), nil, &event); err != nil {
			return nil, err
		}
	} else {
		found, err := a.lookupRequest(ctx, token, in)
		if err != nil || found == nil {
			return nil, err
		}
		event = *found
	}
	return matchingReceipt(event, in)
}

//nolint:nilnil // A mismatched saved interval requires an update, not a second create.
func matchingReceipt(event scheduledEvent, in connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	if event.ID == "" || (!in.Cancel && event.Canceled) || (in.EventID != "" && event.ID != in.EventID) {
		return nil, fmt.Errorf("calendar: event identity is unavailable")
	}
	if in.Cancel {
		return &connector.CalendarReceipt{EventID: event.ID}, nil
	}
	start, err := graphInstant(event.Start)
	if err != nil {
		return nil, err
	}
	end, err := graphInstant(event.End)
	if err != nil {
		return nil, err
	}
	if !start.Equal(in.Start) || !end.Equal(in.End) {
		if in.EventID != "" {
			return nil, nil
		}
		return nil, fmt.Errorf("calendar: existing invitation has changed; reconcile before retrying")
	}
	return &connector.CalendarReceipt{EventID: event.ID, UID: event.UID, URL: event.URL}, nil
}

func (a *httpAPI) Inspect(ctx context.Context, token, calendar, event string) (connector.CalendarState, error) {
	var out scheduledEvent
	status, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, a.base+"/me/events/"+url.PathEscape(event), nil, &out)
	if status == http.StatusNotFound {
		return connector.CalendarState{Canceled: true}, nil
	}
	if err != nil {
		return connector.CalendarState{}, err
	}
	if out.ID != event {
		return connector.CalendarState{}, fmt.Errorf("calendar: mismatched event identity")
	}
	if out.Canceled {
		return connector.CalendarState{Canceled: true}, nil
	}
	start, err := graphInstant(out.Start)
	if err != nil {
		return connector.CalendarState{}, err
	}
	end, err := graphInstant(out.End)
	return connector.CalendarState{Start: start, End: end}, err
}

//nolint:nilnil // Absence is distinct from provider failure.
func (a *httpAPI) lookupRequest(ctx context.Context, token string, in connector.CalendarAppointment) (*scheduledEvent, error) {
	// Request ids originate in Margince, never in guest input.
	query := url.Values{"$filter": {"singleValueExtendedProperties/Any(ep: ep/id eq '" + meetingRequestProperty + "' and ep/value eq '" + in.RequestID + "')"}, "$top": {"2"}}
	var result struct {
		Events []scheduledEvent `json:"value"`
		Next   string           `json:"@odata.nextLink"`
	}
	if _, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, a.base+calendarPath(in.CalendarID)+"/events?"+query.Encode(), nil, &result); err != nil {
		return nil, err
	}
	if len(result.Events) == 0 && result.Next == "" {
		return nil, nil
	}
	if len(result.Events) != 1 || result.Next != "" {
		return nil, fmt.Errorf("calendar: ambiguous invitation identity")
	}
	return &result.Events[0], nil
}
