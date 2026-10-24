// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func callCaptureHealth(p principal.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/capture-health", nil)
	rec := httptest.NewRecorder()
	captureHealthHandlers{}.GetCaptureHealth(rec, req.WithContext(principal.WithActor(req.Context(), p)))
	return rec
}

// A passport minted by an admin satisfies every object grant, so the human
// rung is what keeps an agent off a page about everyone's mailboxes.
func TestCaptureHealthRefusesAnAgentHoldingTheGrant(t *testing.T) {
	rec := callCaptureHealth(principal.Principal{
		Type: principal.PrincipalAgent,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"job_health": {Read: true}},
		},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("an agent got %d, want 403", rec.Code)
	}
}

func TestCaptureHealthRefusesAHumanWithoutTheJobHealthGrant(t *testing.T) {
	rec := callCaptureHealth(principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"contact": {Read: true}},
		},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("a human without job_health got %d, want 403", rec.Code)
	}
}

// A half-recorded gap shows nothing. The two columns carry a CHECK that they
// are set together, so a single end means a row written by something other
// than the sync. A client cannot choose a backfill window from one date.
func TestUnreadPeriodNeedsBothEnds(t *testing.T) {
	at := time.Date(2026, 2, 22, 9, 0, 0, 0, time.UTC)
	later := at.Add(48 * time.Hour)

	for _, c := range []struct {
		name     string
		from, to *time.Time
	}{
		{"neither end", nil, nil},
		{"a start with no end", &at, nil},
		{"an end with no start", nil, &later},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := unreadPeriodOf(c.from, c.to); got != nil {
				t.Errorf("reports the period %v to %v", got.From, got.To)
			}
		})
	}
}

// The wire carries UTC. A mailbox read in a local zone would otherwise shift
// the dates a reader is asked to match a backfill window to.
func TestUnreadPeriodReportsBothEndsInUTC(t *testing.T) {
	zone := time.FixedZone("UTC+7", 7*60*60)
	from := time.Date(2026, 2, 22, 9, 0, 0, 0, zone)
	to := time.Date(2026, 2, 24, 9, 0, 0, 0, zone)

	got := unreadPeriodOf(&from, &to)
	if got == nil {
		t.Fatal("a whole period was not reported")
	}
	if !got.From.Equal(from) || got.From.Location() != time.UTC {
		t.Errorf("the start reads %s, want %s in UTC", got.From, from.UTC())
	}
	if !got.To.Equal(to) || got.To.Location() != time.UTC {
		t.Errorf("the end reads %s, want %s in UTC", got.To, to.UTC())
	}
}
