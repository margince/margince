// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The sync_health lane: the overlay sync's aggregated concerns, absent on a
// workspace not in overlay mode, withheld when the reader may not read the
// sync surface at all.

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type stubSyncHealth struct {
	rows []SyncConcern
	err  error
}

func (s *stubSyncHealth) Concerns(context.Context) ([]SyncConcern, error) {
	return s.rows, s.err
}

func syncLaneService(health SyncHealth) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, health, nil, fixedClock)
}

func TestASyncConcernCarriesItsConditionAndItsFacts(t *testing.T) {
	svc := syncLaneService(&stubSyncHealth{rows: []SyncConcern{
		{Kind: "sync_failing", ErrorClass: "auth", Failures: 4},
		{Kind: "budget_degraded", Band: "shed"},
		{Kind: "objects_stale", Objects: []string{"deal", "person"}},
	}})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.SyncHealth == nil || len(*out.SyncHealth) != 3 {
		t.Fatalf("sync_health carries %v, want three concerns", out.SyncHealth)
	}
	if out.Counts.SyncHealth == nil || *out.Counts.SyncHealth != 3 {
		t.Fatalf("sync_health count = %v, want 3", out.Counts.SyncHealth)
	}
	for i, want := range []struct{ kind, detail string }{
		{"sync_failing", "auth"},
		{"budget_degraded", "shed"},
		{"objects_stale", "deal, person"},
	} {
		item := (*out.SyncHealth)[i]
		if item.Source != crmcontracts.AttentionItemSource("sync_health") {
			t.Errorf("item %d source = %q, want sync_health", i, item.Source)
		}
		if item.Kind == nil || *item.Kind != want.kind {
			t.Errorf("item %d kind = %v, want %q", i, item.Kind, want.kind)
		}
		if item.Detail == nil || *item.Detail != want.detail {
			t.Errorf("item %d detail = %v, want %q — the card's facts did not travel", i, item.Detail, want.detail)
		}
		if item.Id != want.kind {
			t.Errorf("item %d id = %q, want its kind %q — a concern is a condition, not a row", i, item.Id, want.kind)
		}
		if len(item.Actions) != 0 {
			t.Errorf("item %d offers %v — the card promises no verbs", i, item.Actions)
		}
	}
}

// A workspace not in overlay mode has no sync to be healthy about: the lane is
// ABSENT — not empty, and not named as withheld, because nothing was hidden.
func TestANonOverlayWorkspaceHasNoSyncHealthLane(t *testing.T) {
	svc := syncLaneService(&stubSyncHealth{err: apperrors.ErrModeNotOverlay})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.SyncHealth != nil {
		t.Fatalf("sync_health = %v, want the lane absent on a non-overlay workspace", out.SyncHealth)
	}
	if out.LanesOmitted != nil && slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("sync_health")) {
		t.Fatal("sync_health named as withheld — but nothing was hidden from this reader")
	}
}

func TestARefusedSyncHealthReadIsNamedAsWithheld(t *testing.T) {
	svc := syncLaneService(&stubSyncHealth{err: apperrors.ErrPermissionDenied})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.SyncHealth != nil {
		t.Fatalf("sync_health = %v, want the lane withheld", out.SyncHealth)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("sync_health")) {
		t.Fatal("a refused sync_health read was not named in lanes_omitted")
	}
}

// Healthy sync is an EMPTY lane, not an absent one: the feed looked and found
// nothing wrong, which is a promise the absent lane never makes.
func TestAHealthySyncReadsAsAnEmptyLane(t *testing.T) {
	svc := syncLaneService(&stubSyncHealth{})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.SyncHealth == nil || len(*out.SyncHealth) != 0 {
		t.Fatalf("sync_health = %v, want an empty lane on a healthy sync", out.SyncHealth)
	}
	if out.Counts.SyncHealth == nil || *out.Counts.SyncHealth != 0 {
		t.Fatalf("sync_health count = %v, want 0", out.Counts.SyncHealth)
	}
}
