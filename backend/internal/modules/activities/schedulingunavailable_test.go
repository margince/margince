// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const sentenceForTheHost = "calendar chosen in scheduling settings"

// However the provider fails to describe the calendar, the answer names the
// calendar as the problem. The log names which calendar and why. The test goes
// through httperr.Write, so an opaque 500 fails it.
func TestAnUnreadableCalendarIsAnswered502NamingTheCalendarAndLogsWhy(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	for name, tc := range map[string]struct {
		err        error
		wantReason string
	}{
		"the provider names an error": {connector.NewCalendarUnavailable("team@example.test", "notFound", nil), "notFound"},
		"the provider leaves it out":  {connector.NewCalendarUnavailable("team@example.test", "omitted", nil), "omitted"},
		"an incomplete answer":        {errors.New("calendar: incomplete availability"), ""},
		"a malformed interval":        {errors.New("calendar: invalid busy interval"), ""},
		"an unreachable provider":     {fmt.Errorf("calendar: provider status 503: %w", connector.ErrUnreachable), ""},
	} {
		logged.Reset()
		err := calendarReadFailure(context.Background(), ids.New[ids.UserKind](), "gcal", "team@example.test", tc.err)

		rec := httptest.NewRecorder()
		httperr.Write(rec, httptest.NewRequest(http.MethodGet, "/v1/availability", nil), err)
		var body struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		}
		if decodeErr := json.Unmarshal(rec.Body.Bytes(), &body); decodeErr != nil {
			t.Fatalf("%s: %v", name, decodeErr)
		}
		if rec.Code != http.StatusBadGateway || body.Code != connector.CalendarUnavailableCode || !strings.Contains(body.Detail, sentenceForTheHost) {
			t.Errorf("%s answered %d %s, want 502 %s pointing at the calendar choice", name, rec.Code, rec.Body.String(), connector.CalendarUnavailableCode)
		}
		if strings.Contains(rec.Body.String(), "team@example.test") {
			t.Errorf("%s: the host's calendar id reached the client: %s", name, rec.Body.String())
		}
		line := logged.String()
		for _, want := range []string{"level=WARN", "team@example.test", "provider=gcal", "host_id="} {
			if !strings.Contains(line, want) {
				t.Errorf("%s: log %q lacks %s", name, line, want)
			}
		}
		if tc.wantReason != "" && !strings.Contains(line, "reason="+tc.wantReason) {
			t.Errorf("%s: log %q does not carry the provider's reason %s", name, line, tc.wantReason)
		}
		if !errors.Is(err, apperrors.ErrProviderUnusable) {
			t.Errorf("%s: a job reading the shared vocabulary cannot tell the provider gave no answer", name)
		}
	}
}

// A refused credential has its own answer, and a permission refusal is not the
// calendar's fault. A caller who left must reach httperr's client-closed
// branch, not be told the calendar failed.
func TestACalendarReadFailureThatIsNotTheCalendarsKeepsItsOwnAnswer(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, tc := range map[string]struct {
		ctx context.Context
		err error
	}{
		"a refused credential": {context.Background(), connector.ErrAuthRejected},
		"a permission refusal": {context.Background(), apperrors.ErrPermissionDenied},
		"a caller who left":    {cancelled, context.Canceled},
	} {
		got := calendarReadFailure(tc.ctx, ids.New[ids.UserKind](), "gcal", "primary", tc.err)
		if !errors.Is(got, tc.err) {
			t.Errorf("%s was rewritten as %v", name, got)
		}
		if _, rewritten := errors.AsType[*connector.CalendarUnavailableError](got); rewritten {
			t.Errorf("%s was reported as an unreadable calendar", name)
		}
	}
}
