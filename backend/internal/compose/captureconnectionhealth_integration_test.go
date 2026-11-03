// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAdminReadsAnotherMembersStalledConnection(t *testing.T) {
	b := setupBackfillWire(t)
	e := b.env
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	views, err := b.registry.Connections(b.human)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) == 0 {
		t.Fatal("registry returned no connected mailbox")
	}
	id := views[0].ID
	for _, v := range views {
		if v.Provider == "gmail" {
			id = v.ID
		}
	}
	e.WsExec(t, `UPDATE capture_connection SET connected_at = $1 WHERE id = $2`, at.Add(-48*time.Hour), id)
	e.WsExec(t, `INSERT INTO capture_sync_state (connection_id, last_success_at) VALUES ($1,$2)
 ON CONFLICT (connection_id) DO UPDATE SET last_success_at=EXCLUDED.last_success_at`, id, at.Add(-25*time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/capture-health", nil).WithContext(e.Admin())
	rec := httptest.NewRecorder()
	captureHealthHandlers{pool: e.Pool, now: func() time.Time { return at }}.GetCaptureHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health = %d: %s", rec.Code, rec.Body.String())
	}
	var health crmcontracts.CaptureHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, concern := range health.ConnectionConcerns {
		if concern.ConnectionId.String() != id.String() {
			continue
		}
		found = true
		if concern.UserId.String() != e.Rep1.String() || concern.Concern != crmcontracts.CaptureConcernKindSyncStalled || concern.LastSuccessAt == nil || !concern.LastSuccessAt.Equal(at.Add(-25*time.Hour)) {
			t.Fatalf("concern = %+v", concern)
		}
	}
	if !found {
		t.Fatalf("stalled connection absent: %+v", health.ConnectionConcerns)
	}
	source, findings, err := (Server{captureHealthHandlers: captureHealthHandlers{pool: e.Pool}}).captureSummary(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	overview := false
	for _, finding := range findings {
		if finding.Code == "capture_connection_sync_stalled" {
			overview = true
			if !finding.RequiresAction || finding.Count == nil || *finding.Count != 1 {
				t.Fatalf("overview concern: %+v", finding)
			}
		}
	}
	if !overview {
		t.Fatalf("connection absent from overview: source=%+v findings=%+v", source, findings)
	}
	b.registry.WithClock(func() time.Time { return at })
	if _, err := b.registry.DismissSilence(b.human, "gmail", at.Add(-48*time.Hour)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stalled dismissal = %v, want conflict", err)
	}
	other := e.As(e.Rep2, nil, principal.Permissions{})
	if _, err := b.registry.DismissSilence(other, "gmail", at); !errors.Is(err, capture.ErrNoConnection) {
		t.Fatalf("another member's dismissal = %v", err)
	}
}

func TestDismissSilenceHandlerPreservesItsErrorContract(t *testing.T) {
	b := setupBackfillWire(t)
	for _, tc := range []struct {
		name, body string
		other      bool
		want       int
	}{
		{"another owner", `{"since":"2026-10-01T09:00:00Z"}`, true, http.StatusNotFound},
		{"changed episode", `{"since":"2026-10-01T09:00:00Z"}`, false, http.StatusConflict},
		{"invalid timestamp", `{"since":"not-a-date"}`, false, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b.human
			if tc.other {
				ctx = b.env.As(b.env.Rep2, nil, principal.Permissions{})
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/connectors/gmail/silence/dismiss", strings.NewReader(tc.body)).WithContext(ctx)
			rec := httptest.NewRecorder()
			connectorHandlers{registry: b.registry}.DismissConnectorSilence(rec, req, crmcontracts.CaptureProviderGmail)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}
