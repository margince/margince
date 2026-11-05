// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestGoogleAvailabilitySkipsAnEmptyEventAmongBusyEvents(t *testing.T) {
	server := serveGoogle(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"timeZone":"UTC","items":[{"id":"empty","start":{"dateTime":"2026-10-05T10:00:00Z"},"end":{"dateTime":"2026-10-05T10:00:00Z"}},{"id":"busy","start":{"dateTime":"2026-10-05T11:00:00Z"},"end":{"dateTime":"2026-10-05T12:00:00Z"}}]}`)); err != nil {
			t.Error(err)
		}
	})
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	from := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	busy, err := api.Busy(context.Background(), "token", teamCalendar, from, from.Add(24*time.Hour))
	if err != nil || len(busy) != 1 || busy[0].EventID != "busy" || busy[0].End.Sub(busy[0].Start) != time.Hour {
		t.Fatalf("busy=%+v err=%v", busy, err)
	}
}
