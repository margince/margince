// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGraphAvailabilitySkipsAnEmptyEventAmongBusyEvents(t *testing.T) {
	events := []scheduledEvent{
		{ID: "empty", ShowAs: "busy", Start: scheduledTime{"2026-10-05T10:00:00", "UTC"}, End: scheduledTime{"2026-10-05T10:00:00", "UTC"}},
		{ID: "busy", ShowAs: "busy", Start: scheduledTime{"2026-10-05T11:00:00", "UTC"}, End: scheduledTime{"2026-10-05T12:00:00", "UTC"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(struct {
			Value []scheduledEvent `json:"value"`
		}{events}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	from := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	busy, err := api.Busy(context.Background(), "token", "primary", from, from.Add(24*time.Hour))
	if err != nil || len(busy) != 1 || busy[0].EventID != "busy" || busy[0].End.Sub(busy[0].Start) != time.Hour {
		t.Fatalf("busy=%+v err=%v", busy, err)
	}
}
