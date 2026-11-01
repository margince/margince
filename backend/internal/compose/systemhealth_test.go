// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func healthTestContext(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:test", Permissions: principal.Permissions{Objects: objects}})
}

func TestHealthCompositionNeverReadsAWithheldSource(t *testing.T) {
	ctx := healthTestContext(map[string]principal.ObjectGrant{"embedding_reindex": {Read: true}})
	reads := []healthRead{
		{"jobs", "job_health", func(context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
			t.Fatal("read without permission")
			return crmcontracts.HealthSource{}, nil, nil
		}},
		{"search", "embedding_reindex", func(context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
			s := healthSource("search", "embedding_reindex")
			s.Condition = "clear"
			return s, nil, nil
		}},
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	report := composeHealth(ctx, reads, func() time.Time { return at })
	if report.Sources[0].Availability != "withheld" || report.Sources[0].ObservedAt != nil || report.Sources[0].Count != nil {
		t.Fatalf("withheld source leaked evidence: %+v", report.Sources[0])
	}
	if report.Sources[1].Condition != "clear" || !report.Complete {
		t.Fatalf("permitted search was not returned: %+v", report)
	}
}

func TestHealthCompositionKeepsReadableEvidenceBesideAFailedSource(t *testing.T) {
	ctx := healthTestContext(map[string]principal.ObjectGrant{"job_health": {Read: true}})
	reads := []healthRead{
		{"jobs", "job_health", func(context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
			return crmcontracts.HealthSource{}, nil, errors.New("synthetic source unavailable")
		}},
		{"capture", "job_health", func(context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
			s := healthSource("capture", "job_health")
			s.Condition = "technical"
			f := healthFinding(s, "capture_exhausted", "technical", 3, "records")
			return s, []crmcontracts.HealthFinding{f}, nil
		}},
	}
	report := composeHealth(ctx, reads, func() time.Time { return time.Time{} })
	if report.Assessment != "attention" || report.Complete || len(report.Findings) != 2 || report.ActionCount != 2 || report.Sources[0].Availability != "failed" {
		t.Fatalf("partial evidence misrepresented: %+v", report)
	}
}

func TestHealthCompositionCannotTurnAnUnknownSourceIntoAllClear(t *testing.T) {
	ctx := healthTestContext(map[string]principal.ObjectGrant{"ai_diagnostics": {Read: true}})
	reads := []healthRead{{"providers", "ai_diagnostics", func(context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
		s := healthSource("providers", "ai_diagnostics")
		s.Availability = "unknown"
		return s, nil, nil
	}}}
	report := composeHealth(ctx, reads, func() time.Time { return time.Time{} })
	if report.Assessment != "incomplete" || report.Complete {
		t.Fatalf("unknown became clear: %+v", report)
	}
}

func TestSystemHealthRequiresAHumanAndAnExistingPageGrant(t *testing.T) {
	for _, actor := range []principal.Principal{
		{Type: principal.PrincipalHuman},
		{Type: principal.PrincipalSystem},
		{Type: principal.PrincipalConnector, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"job_health": {Read: true}}}},
		{Type: principal.PrincipalAgent, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"job_health": {Read: true}}}},
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/system-health", nil)
		rec := httptest.NewRecorder()
		Server{}.GetSystemHealth(rec, req.WithContext(principal.WithActor(req.Context(), actor)))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("got %d for %s", rec.Code, actor.Type)
		}
	}
}

func TestHealthSourceBoundCoversEveryRegisteredReader(t *testing.T) {
	raw, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	type boundedArray struct {
		MaxItems int `yaml:"maxItems"` //nolint:tagliatelle // OpenAPI schema keywords use camel case.
	}
	type properties struct {
		Properties map[string]boundedArray `yaml:"properties"`
	}
	var document struct {
		Components struct {
			Schemas map[string]properties `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	bound := document.Components.Schemas["SystemHealth"].Properties["sources"].MaxItems
	reads := Server{}.healthReads()
	if bound == 0 || len(reads) > bound {
		t.Fatalf("%d readers exceed the summary contract bound", len(reads))
	}
	seen := map[string]bool{}
	for _, read := range reads {
		if read.id == "" || seen[read.id] {
			t.Fatalf("source identity is not unique: %q", read.id)
		}
		seen[read.id] = true
	}
}

func TestHealthFindingOrderingIsTransitiveWithMissingCounts(t *testing.T) {
	source := healthSource("jobs", "job_health")
	findings := []crmcontracts.HealthFinding{
		healthFinding(source, "a", "technical", 1, "runs"),
		healthFinding(source, "b", "technical", 2, "runs"),
		healthFinding(source, "c", "technical", 3, "runs"),
	}
	findings[1].Count = nil
	for _, a := range findings {
		for _, b := range findings {
			for _, c := range findings {
				if healthFindingLess(a, b) && healthFindingLess(b, c) && !healthFindingLess(a, c) {
					t.Fatalf("nontransitive: %s < %s < %s", a.Id, b.Id, c.Id)
				}
			}
		}
	}
}

func TestIntakeWindowUsesTheUTCReportingDay(t *testing.T) {
	at := time.Date(2026, 2, 1, 0, 30, 0, 0, time.FixedZone("reader", 7*60*60))
	want := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)
	if got := extensionIngestWindowStart(at); !got.Equal(want) {
		t.Fatalf("window starts %s, want %s", got, want)
	}
}

func TestHealthActionsExcludeAutomaticProcessing(t *testing.T) {
	source := healthSource("jobs", "job_health")
	for _, condition := range []crmcontracts.HealthSourceCondition{"clear", "processing", "retrying", "unknown", "recovered"} {
		if healthFinding(source, "queue_checked", condition, 1, "runs").RequiresAction {
			t.Fatalf("%s unexpectedly requires human action", condition)
		}
	}
	for _, condition := range []crmcontracts.HealthSourceCondition{"technical", "waiting_human", "unverified"} {
		if !healthFinding(source, "queue_overdue", condition, 1, "runs").RequiresAction {
			t.Fatalf("%s lost its next action", condition)
		}
	}
}

func TestJobSummaryKeepsLiveRetriesBesideDiscardedHistory(t *testing.T) {
	next := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	age := 600.0
	for _, row := range []jobs.KindHealth{
		{Kind: "capture_classify", Discarded: 12, FutureRetries: 2, Retrying: 2, NextAttemptAt: &next},
		{Kind: "unregistered_kind", Discarded: 12, FutureRetries: 2, Retrying: 2, NextAttemptAt: &next, Waiting: 1, Runnable: 1, OldestWaitingAgeSeconds: &age},
		{Kind: "unregistered_kind", FutureRetries: 2, Retrying: 2, NextAttemptAt: &next, Waiting: 1, Runnable: 1, OldestWaitingAgeSeconds: &age},
	} {
		_, findings, err := summarizeJobKinds(healthSource("jobs", healthReadObject), []jobs.KindHealth{row})
		if err != nil {
			t.Fatal(err)
		}
		counts := map[string]int{}
		for _, finding := range findings {
			if finding.Count != nil {
				counts[finding.Code] = *finding.Count
			}
		}
		if counts["queue_unverified"] != int(row.Discarded) || counts["queue_retrying"] != 2 {
			t.Fatalf("history hid current retries: %+v", findings)
		}
	}
}
