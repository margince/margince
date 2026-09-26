// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//nolint:tagliatelle // Calendar provider field names are an external wire contract.
package gcal

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/margince/margince/backend/internal/modules/capture/calendarwire"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	calendarSingleEvents = "singleEvents"
	calendarMaxResults   = "maxResults"
	calendarCanceled     = "cancelled"
)

func (a *httpAPI) List(ctx context.Context, token string) ([]connector.CalendarOption, error) {
	query := url.Values{calendarMaxResults: {"250"}, "minAccessRole": {"reader"}}
	calendars := []connector.CalendarOption{}
	for page := 0; page < 20; page++ {
		var result struct {
			Next  string `json:"nextPageToken"`
			Items []struct {
				ID      string `json:"id"`
				Summary string `json:"summary"`
				Role    string `json:"accessRole"`
				Primary bool   `json:"primary"`
				Deleted bool   `json:"deleted"`
			} `json:"items"`
		}
		if _, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, a.base+"/users/me/calendarList?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			if !item.Deleted {
				calendars = append(calendars, connector.CalendarOption{ID: item.ID, Name: item.Summary, Writable: item.Role == "owner", Primary: item.Primary})
			}
		}
		if result.Next == "" {
			return calendars, nil
		}
		query.Set("pageToken", result.Next)
	}
	return nil, fmt.Errorf("calendar: incomplete calendar list")
}
