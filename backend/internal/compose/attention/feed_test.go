// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// The feed's promise is that a rep can read it and know what their day holds.
// Each test below is one way that promise can be broken.

var readInstant = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return readInstant }

type stubApprovals struct {
	rows []crmcontracts.Approval
	err  error
}

func (s stubApprovals) ListWire(context.Context, ApprovalQuery) ([]crmcontracts.Approval, error) {
	return s.rows, s.err
}

type stubDuplicates struct {
	pairs []DuplicatePair
	open  int
	err   error
}

func (s stubDuplicates) OpenCandidates(context.Context, int) ([]DuplicatePair, error) {
	return s.pairs, s.err
}

func (s stubDuplicates) CountOpen(context.Context) (int, error) { return s.open, s.err }

type stubTasks struct {
	rows []Task
	err  error
}

func (s stubTasks) OpenForViewer(context.Context, time.Time, int) ([]Task, error) {
	return s.rows, s.err
}

type stubReceipts struct {
	rows []Receipt
	err  error
}

func (s stubReceipts) Recent(context.Context, time.Time, int) ([]Receipt, error) {
	return s.rows, s.err
}

func approval(summary string) crmcontracts.Approval {
	return crmcontracts.Approval{
		Id:      openapi_types.UUID(ids.NewV7()),
		Kind:    "send_email",
		Summary: &summary,
	}
}

func TestADuplicateOutranksAnApprovalBecauseAMergeCannotBeUndone(t *testing.T) {
	svc := NewService(
		stubApprovals{rows: []crmcontracts.Approval{approval("Send the Weber follow-up")}},
		stubDuplicates{pairs: []DuplicatePair{{ID: ids.NewV7(), EntityType: "person", Confidence: 0.9}}, open: 1},
		stubTasks{}, stubReceipts{}, fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(out.NeedsYou) != 2 {
		t.Fatalf("needs_you carries %d items, want 2", len(out.NeedsYou))
	}
	if got := out.NeedsYou[0].Source; got != "dedupe_candidate" {
		t.Errorf("the lane leads with %q; a merge has no undo, so it outranks a reversible proposal", got)
	}
}

func TestAWithheldLaneIsNamedRatherThanReportedEmpty(t *testing.T) {
	svc := NewService(
		stubApprovals{},
		stubDuplicates{err: apperrors.ErrPermissionDenied},
		stubTasks{rows: []Task{{ID: ids.NewV7(), Subject: "Call Anna"}}},
		stubReceipts{}, fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("a refused lane must not fail the read: %v", err)
	}
	if out.LanesOmitted == nil {
		t.Fatal("the refused lane is not named — a rep would read a clear day the server cannot see")
	}
	if got := (*out.LanesOmitted)[0]; got != "needs_you" {
		t.Errorf("omitted lane is %q, want needs_you", got)
	}
	if len(out.NeedsYou) != 0 {
		t.Error("a withheld lane must carry no items")
	}
	if len(out.Planned) != 1 {
		t.Error("one lane being withheld must not cost the reader the others")
	}
}

func TestABrokenLaneFailsTheReadRatherThanReadingAsQuiet(t *testing.T) {
	svc := NewService(
		stubApprovals{err: fmt.Errorf("the database is unreachable")},
		stubDuplicates{}, stubTasks{}, stubReceipts{}, fixedClock,
	)
	if _, err := svc.Assemble(context.Background()); err == nil {
		t.Fatal("a lane that FAILED was reported as an empty day")
	}
}

func TestTheCountReportsTheTotalThoughTheLaneIsBounded(t *testing.T) {
	pairs := make([]DuplicatePair, needsYouCap+5)
	for i := range pairs {
		pairs[i] = DuplicatePair{ID: ids.NewV7(), EntityType: "person", Confidence: 0.8}
	}
	svc := NewService(
		stubApprovals{},
		stubDuplicates{pairs: pairs, open: 40},
		stubTasks{}, stubReceipts{}, fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(out.NeedsYou) != needsYouCap {
		t.Errorf("the lane shows %d items, want it bounded at %d", len(out.NeedsYou), needsYouCap)
	}
	if out.Counts.NeedsYou != 40 {
		t.Errorf("the count says %d; a bounded lane must still report the true total", out.Counts.NeedsYou)
	}
}

func TestAnOverdueTaskLeadsThePlannedLane(t *testing.T) {
	yesterday := readInstant.Add(-24 * time.Hour)
	later := readInstant.Add(6 * time.Hour)
	svc := NewService(
		stubApprovals{}, stubDuplicates{},
		stubTasks{rows: []Task{
			{ID: ids.NewV7(), Subject: "Due later today", DueAt: &later},
			{ID: ids.NewV7(), Subject: "Was due yesterday", DueAt: &yesterday},
		}},
		stubReceipts{}, fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if *out.Planned[0].Title != "Was due yesterday" {
		t.Errorf("planned leads with %q; a promise already broken outranks one merely due", *out.Planned[0].Title)
	}
	if out.Planned[0].Overdue == nil || !*out.Planned[0].Overdue {
		t.Error("overdue is resolved server-side so every surface agrees where today ends")
	}
	if out.Planned[1].Overdue == nil || *out.Planned[1].Overdue {
		t.Error("a task due later today is not overdue")
	}
}

func TestAReceiptOffersNoDecision(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, stubTasks{},
		stubReceipts{rows: []Receipt{{
			ID: ids.NewV7(), Kind: "close_date_correction",
			Summary: "Moved the Acme close date to 27 Sep", OccurredAt: readInstant.Add(-time.Hour),
		}}},
		fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(out.DoneForYou) != 1 {
		t.Fatalf("done_for_you carries %d items, want 1", len(out.DoneForYou))
	}
	for _, action := range out.DoneForYou[0].Actions {
		if action == "decide" || action == "merge" {
			t.Errorf("a finished act offers %q — it would ask a reader to answer a settled question", action)
		}
	}
}

func TestADuplicateCarriesNoServerWrittenSentence(t *testing.T) {
	svc := NewService(
		stubApprovals{},
		stubDuplicates{pairs: []DuplicatePair{{ID: ids.NewV7(), EntityType: "organization", Confidence: 0.92}}, open: 1},
		stubTasks{}, stubReceipts{}, fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	item := out.NeedsYou[0]
	// The product ships three languages. A sentence composed in Go reaches a
	// German reader in English, so the client is handed the facts and writes
	// the line itself.
	if item.Title != nil {
		t.Errorf("the server wrote %q for a duplicate; that sentence has no translation", *item.Title)
	}
	if item.Kind == nil || *item.Kind != "organization" {
		t.Error("the client needs the record type to write the line")
	}
	if item.Confidence == nil {
		t.Error("the client needs the confidence to qualify the claim")
	}
}
