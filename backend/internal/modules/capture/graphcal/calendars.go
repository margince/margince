// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//nolint:tagliatelle // Calendar provider field names are an external wire contract.
package graphcal

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture/calendarwire"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (a *httpAPI) List(ctx context.Context, token string) ([]connector.CalendarOption, error) {
	next := a.base + "/me/calendars?$select=id,name,canEdit,isDefaultCalendar&$top=100"
	calendars := []connector.CalendarOption{}
	for page := 0; page < 20; page++ {
		var result struct {
			Next  string `json:"@odata.nextLink"`
			Items []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Writable bool   `json:"canEdit"`
				Primary  bool   `json:"isDefaultCalendar"`
			} `json:"value"`
		}
		if _, err := calendarwire.Request(ctx, a.client, token, http.MethodGet, next, nil, &result); err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			calendars = append(calendars, connector.CalendarOption{ID: item.ID, Name: item.Name, Writable: item.Writable, Primary: item.Primary})
		}
		if result.Next == "" {
			return calendars, nil
		}
		if !strings.HasPrefix(result.Next, a.base+"/") {
			return nil, fmt.Errorf("calendar: invalid continuation")
		}
		next = result.Next
	}
	return nil, fmt.Errorf("calendar: incomplete calendar list")
}
