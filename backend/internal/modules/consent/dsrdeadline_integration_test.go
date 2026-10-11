// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
)

// deadlinePack is a jurisdiction whose law gives access requests 72 hours and
// leaves every other kind to the GDPR month, under a reserved code.
const deadlinePack jurisdiction.Code = "zd"

func init() {
	subjectrightsrules.Register(subjectrightsrules.Rules{
		Jurisdiction: deadlinePack, Version: 3,
		Instruments: []messagingrules.Instrument{{Name: "Test Act 1/2026"}},
		Deadlines:   map[subjectrightsrules.RequestKind]subjectrightsrules.Deadline{"access": {Hours: 72}},
	})
}

func (e *dsrEnv) underJurisdiction(code jurisdiction.Code) Handlers {
	country := InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return code, nil })
	return e.handlers().WithInstallationCountry(country)
}

func (e *dsrEnv) file(t *testing.T, h Handlers, body string) (int, crmcontracts.DataSubjectRequest, string) {
	t.Helper()
	w := e.sendDSR(http.MethodPost, body, func(w http.ResponseWriter, r *http.Request) {
		h.CreateDataSubjectRequest(w, r, crmcontracts.CreateDataSubjectRequestParams{})
	})
	var out crmcontracts.DataSubjectRequest
	if w.Code == http.StatusCreated {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding the created request: %v", err)
		}
		return w.Code, out, ""
	}
	return w.Code, out, validationField(t, w.Body.Bytes())
}

// A request filed without a date gets the deadline the selected jurisdiction
// gives its kind, counted from when it arrived, and names the rule.
func TestAManualRequestGetsTheComputedDeadline(t *testing.T) {
	e := setupDSR(t)
	received := time.Date(2026, time.January, 31, 9, 0, 0, 0, time.UTC)
	at := received.Format(time.RFC3339)

	status, gdpr, _ := e.file(t, e.handlers(), `{"kind":"erasure","subject_ref":"a@dsr.test","received_at":"`+at+`"}`)
	if status != http.StatusCreated || !gdpr.DueAt.Equal(time.Date(2026, time.February, 28, 9, 0, 0, 0, time.UTC)) ||
		gdpr.DueRule == nil || *gdpr.DueRule != "gdpr" || gdpr.DueOverridden {
		t.Fatalf("no jurisdiction → %d due %s rule %v overridden %v; want the GDPR month clamped to 28 February",
			status, gdpr.DueAt, gdpr.DueRule, gdpr.DueOverridden)
	}

	status, pack, _ := e.file(t, e.underJurisdiction(deadlinePack), `{"kind":"access","subject_ref":"b@dsr.test","received_at":"`+at+`"}`)
	if status != http.StatusCreated || !pack.DueAt.Equal(received.Add(72*time.Hour)) || pack.DueRule == nil || *pack.DueRule != "zd v3" {
		t.Fatalf("zd access → %d due %s rule %v; want 72 hours under zd v3", status, pack.DueAt, pack.DueRule)
	}
	status, fallback, _ := e.file(t, e.underJurisdiction(deadlinePack), `{"kind":"rectify","subject_ref":"c@dsr.test","received_at":"`+at+`"}`)
	if status != http.StatusCreated || fallback.DueRule == nil || *fallback.DueRule != "gdpr" {
		t.Fatalf("a kind the pack declares nothing for → %d rule %v; want the GDPR fallback", status, fallback.DueRule)
	}
}

// A deadline later than the law's would show a request as on time on a day it
// is already late. It is refused, and nothing is stored.
func TestADeadlineLaterThanTheComputedOneIsRefused(t *testing.T) {
	e := setupDSR(t)
	received := time.Now().UTC().Add(-time.Hour)
	late := received.Add(73 * time.Hour).Format(time.RFC3339)
	status, _, field := e.file(t, e.underJurisdiction(deadlinePack),
		`{"kind":"access","subject_ref":"late@dsr.test","received_at":"`+received.Format(time.RFC3339)+`","due_at":"`+late+`"}`)
	if status != http.StatusUnprocessableEntity || field != fieldDueAt {
		t.Fatalf("a due_at past the 72-hour deadline → %d naming %q, want 422 naming due_at", status, field)
	}
	rows, _, err := e.store.ListDSRs(e.ctx, nil, "", "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("a refused request was stored anyway: %d rows, %v", len(rows), err)
	}

	early := received.Add(24 * time.Hour).Truncate(time.Second)
	status, stored, _ := e.file(t, e.underJurisdiction(deadlinePack),
		`{"kind":"access","subject_ref":"early@dsr.test","received_at":"`+received.Format(time.RFC3339)+`","due_at":"`+early.Format(time.RFC3339)+`"}`)
	if status != http.StatusCreated || !stored.DueAt.Equal(early) || !stored.DueOverridden {
		t.Fatalf("an earlier due_at → %d due %s overridden %v; want it stored and marked overridden", status, stored.DueAt, stored.DueOverridden)
	}
}

// A request cannot have arrived in the future: that would grant days the
// subject is not owed.
func TestARequestReceivedInTheFutureIsRefused(t *testing.T) {
	e := setupDSR(t)
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	status, _, field := e.file(t, e.handlers(), `{"kind":"access","subject_ref":"f@dsr.test","received_at":"`+future+`"}`)
	if status != http.StatusUnprocessableEntity || field != fieldReceivedAt {
		t.Fatalf("a future received_at → %d naming %q, want 422 naming received_at", status, field)
	}
}

// The form's preview and the filing come from one resolver.
func TestTheDeadlinePreviewMatchesTheFiling(t *testing.T) {
	e := setupDSR(t)
	received := time.Date(2026, time.March, 31, 12, 0, 0, 0, time.UTC)
	h := e.underJurisdiction(deadlinePack)
	r, err := http.NewRequestWithContext(e.ctx, http.MethodGet, "/v1/data-subject-requests/deadline", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := e.sendDSR(http.MethodGet, "", func(w http.ResponseWriter, _ *http.Request) {
		h.GetDataSubjectRequestDeadline(w, r, crmcontracts.GetDataSubjectRequestDeadlineParams{Kind: "access", ReceivedAt: &received})
	})
	var preview crmcontracts.RequestDeadline
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil || w.Code != http.StatusOK {
		t.Fatalf("preview → %d %s (%v)", w.Code, w.Body, err)
	}
	_, filed, _ := e.file(t, h, `{"kind":"access","subject_ref":"p@dsr.test","received_at":"`+received.Format(time.RFC3339)+`"}`)
	if !preview.DueAt.Equal(filed.DueAt) || filed.DueRule == nil || preview.Rule != *filed.DueRule {
		t.Fatalf("preview %s/%s, filing %s/%v; want one answer", preview.DueAt, preview.Rule, filed.DueAt, filed.DueRule)
	}
}
