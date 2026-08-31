// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The ai_work_health lane: the reader's own troubled AI runs, withheld for a
// caller with no human behind it.

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubAIWork struct {
	rows  []TroubledRun
	err   error
	since time.Time
}

func (s *stubAIWork) Troubled(_ context.Context, since time.Time, _ int) ([]TroubledRun, error) {
	s.since = since
	return s.rows, s.err
}

func aiWorkLaneService(work AIWork) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, work, nil, nil, nil, nil, fixedClock)
}

func TestATroubledRunCarriesItsOwnWordsAndItsSubject(t *testing.T) {
	failedAt := readInstant.Add(-2 * time.Hour)
	stub := &stubAIWork{rows: []TroubledRun{
		{ID: ids.NewV7(), State: "stalled", SubjectLabel: "Weber GmbH", OccurredAt: readInstant.Add(-time.Hour)},
		{ID: ids.NewV7(), State: "failed", Summary: "I could not finish reading the attachment.", OccurredAt: failedAt},
	}}
	out, err := aiWorkLaneService(stub).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AiWorkHealth == nil || len(*out.AiWorkHealth) != 2 {
		t.Fatalf("ai_work_health carries %v, want two runs", out.AiWorkHealth)
	}
	if out.Counts.AiWorkHealth == nil || *out.Counts.AiWorkHealth != 2 {
		t.Fatalf("count = %v, want 2", out.Counts.AiWorkHealth)
	}
	// The window is the feed's own day, ending at the read instant.
	if want := readInstant.Add(-24 * time.Hour); !stub.since.Equal(want) {
		t.Errorf("window opens at %v, want %v", stub.since, want)
	}
	stalled := (*out.AiWorkHealth)[0]
	if stalled.Kind == nil || *stalled.Kind != "stalled" {
		t.Errorf("kind = %v, want the closed failed/stalled vocabulary", stalled.Kind)
	}
	if stalled.Title != nil {
		t.Errorf("a run with no summary invented the headline %q", *stalled.Title)
	}
	if stalled.Detail == nil || *stalled.Detail != "Weber GmbH" {
		t.Errorf("detail = %v, want the subject label", stalled.Detail)
	}
	failed := (*out.AiWorkHealth)[1]
	if failed.Title == nil || *failed.Title != "I could not finish reading the attachment." {
		t.Errorf("title = %v, want the run's own summary", failed.Title)
	}
	if failed.OccurredAt == nil || !failed.OccurredAt.Equal(failedAt) {
		t.Errorf("occurred_at = %v, want when it failed", failed.OccurredAt)
	}
	if len(stalled.Actions)+len(failed.Actions) != 0 {
		t.Error("a troubled-run card promises no verbs")
	}
}

func TestARefusedAIWorkReadIsNamedAsWithheld(t *testing.T) {
	out, err := aiWorkLaneService(&stubAIWork{err: apperrors.ErrPermissionDenied}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AiWorkHealth != nil {
		t.Fatalf("ai_work_health = %v, want the lane withheld", out.AiWorkHealth)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("ai_work_health")) {
		t.Fatal("a refused ai_work_health read was not named in lanes_omitted")
	}
}

// Quiet AI work is an EMPTY lane — the feed looked — which the absent lane
// never promises.
func TestQuietAIWorkReadsAsAnEmptyLane(t *testing.T) {
	out, err := aiWorkLaneService(&stubAIWork{}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AiWorkHealth == nil || len(*out.AiWorkHealth) != 0 {
		t.Fatalf("ai_work_health = %v, want an empty lane", out.AiWorkHealth)
	}
}
