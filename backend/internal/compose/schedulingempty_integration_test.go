// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestGoogleEmptyOccupancyReachesTheComposedBookingPolicy(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"items":[{"id":"primary","summary":"Work","accessRole":"owner","primary":true}]}`
		if strings.HasSuffix(r.URL.Path, "/freeBusy") {
			body = `{"calendars":{"primary":{"busy":[]}}}`
		} else if strings.HasSuffix(r.URL.Path, "/events") {
			body = `{"timeZone":"UTC","items":[{"id":"empty","start":{"dateTime":"2026-10-05T10:00:00Z"},"end":{"dateTime":"2026-10-05T10:00:00Z"}}]}`
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	registry := capture.NewRegistry(e.DB(), nil, identity.NewServiceFor(e.DB()), keyvault.NewMemory())
	registry.Register(gcal.New(&recordingOAuth{}, gcal.NewAPI(server.Client(), server.URL)))
	credential, err := json.Marshal(googleconn.AuthState{RefreshToken: "fixture", Owner: "host@example.test", Granted: []string{"https://www.googleapis.com/auth/calendar.readonly", "https://www.googleapis.com/auth/calendar.events.owned"}})
	if err != nil {
		t.Fatal(err)
	}
	actor, _ := principal.Actor(e.Admin())
	actor.Scopes = principal.NewScopeSet(principal.ScopeRead)
	grantor := principal.WithActor(e.Admin(), actor)
	if _, err := registry.Connect(grantor, "gcal", credential); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	store := e.Activities.WithClock(func() time.Time { return now }).WithSchedulingCalendar(newSchedulingCalendar(e.Pool, registry)).WithWorkingHours(func(context.Context, ids.UserID) (activities.WorkingHours, error) {
		return activities.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: []int{1, 2, 3, 4, 5}, Location: time.UTC}, nil
	})
	if _, err := store.SaveSchedulingProfile(e.Admin(), crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 0, HorizonDays: 30, BufferMinutes: 15, Title: "Discussion"}); err != nil {
		t.Fatal(err)
	}
	slots, _, err := store.ReliableAvailability(e.Admin(), ids.From[ids.UserKind](e.AdminUser), now.Add(3*time.Hour), now.Add(6*time.Hour), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	start := now.Add(4*time.Hour - 15*time.Minute)
	for _, slot := range slots {
		if slot.Start.Equal(start) {
			return
		}
	}
	t.Fatalf("composed availability lost the slot spanning an empty event: %+v", slots)
}
