// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A booking slug is the capability. On a paused page the availability read
// and the booking call answer as an unknown slug does. Status and body match,
// whatever the body sent. Only the profile differs, by design. It
// tells the guest whose page is paused, and nothing more.
func TestAPausedPageAnswersExactlyAsAnUnknownSlug(t *testing.T) {
	e := apptest.SetupBookingApp(t)
	paused := bookingSlug(t, e)
	if status := e.Call(t, http.MethodPut, "/v1/scheduling/profile", map[string]any{
		"enabled": false, "provider": "gcal", "calendar_id": "work", "duration_minutes": 30,
		"notice_minutes": 0, "horizon_days": 30, "buffer_minutes": 0, "title": "Meeting", "location": "",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("pausing the page → %d", status)
	}
	unknown := strings.Repeat("x", len(paused))
	slot := apptest.NextBookableMorning()
	window := "/availability?from=" + slot.Format(time.RFC3339) + "&to=" + slot.Add(8*time.Hour).Format(time.RFC3339)
	complete := `{"start":"` + slot.Format(time.RFC3339) + `","end":"` + slot.Add(30*time.Minute).Format(time.RFC3339) +
		`","booker":{"name":"Guest","email":"guest@example.test"},"consent":{"policy_version":"v1","wording":"ok"}}`
	for name, call := range map[string]struct{ method, path, body string }{
		"availability":    {http.MethodGet, window, ""},
		"malformed body":  {http.MethodPost, "", `{"start":`},
		"incomplete body": {http.MethodPost, "", `{}`},
		"a complete body": {http.MethodPost, "", complete},
	} {
		t.Run(name, func(t *testing.T) {
			pausedStatus, pausedBody := rawPublicCall(t, e, call.method, "/v1/public/booking/"+paused+call.path, call.body)
			unknownStatus, unknownBody := rawPublicCall(t, e, call.method, "/v1/public/booking/"+unknown+call.path, call.body)
			pausedBody = strings.ReplaceAll(pausedBody, paused, unknown)
			if pausedStatus != unknownStatus || pausedBody != unknownBody {
				t.Errorf("paused answered %d %s\nunknown answered %d %s", pausedStatus, pausedBody, unknownStatus, unknownBody)
			}
			if pausedStatus != http.StatusNotFound {
				t.Errorf("paused answered %d, want 404", pausedStatus)
			}
		})
	}
	status, body := rawPublicCall(t, e, http.MethodGet, "/v1/public/booking/"+paused+"/profile", "")
	if status != http.StatusOK || !strings.Contains(body, `"paused":true`) || strings.Contains(body, "enabled") || strings.Contains(body, "video_app") {
		t.Errorf("the paused profile answered %d %s, want host_name and paused only", status, body)
	}
}

// rawPublicCall sends an anonymous request with body sent as is, and answers
// the status and the body as text.
func rawPublicCall(t *testing.T, e *apptest.AppEnv, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, e.TS.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.TS.Client().Do(req) //nolint:bodyclose // closed by apptest.CloseBody below
	if err != nil {
		t.Fatal(err)
	}
	defer apptest.CloseBody(t, resp)
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw)
}
