// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The capture_health lane: the reader's own mailbox connections needing their
// hand, withheld for a caller with no human behind it.

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubCaptureHealth struct {
	rows []CaptureConcern
	err  error
}

func (s *stubCaptureHealth) CaptureConcerns(context.Context) ([]CaptureConcern, error) {
	return s.rows, s.err
}

func captureLaneService(health CaptureHealth) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, health, nil, nil, nil, nil, nil, fixedClock)
}

func TestACaptureConcernNamesTheConditionAndTheMailbox(t *testing.T) {
	labelled := ids.NewV7()
	unlabelled := ids.NewV7()
	svc := captureLaneService(&stubCaptureHealth{rows: []CaptureConcern{
		{ConnectionID: labelled, Kind: "reauth_required", Provider: "gmail", AccountLabel: "rep@example.com"},
		{ConnectionID: unlabelled, Kind: "sync_failing", Provider: "imap"},
	}})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.CaptureHealth == nil || len(*out.CaptureHealth) != 2 {
		t.Fatalf("capture_health carries %v, want two concerns", out.CaptureHealth)
	}
	if out.Counts.CaptureHealth == nil || *out.Counts.CaptureHealth != 2 {
		t.Fatalf("capture_health count = %v, want 2", out.Counts.CaptureHealth)
	}
	first := (*out.CaptureHealth)[0]
	if first.Source != crmcontracts.AttentionItemSource("capture_health") {
		t.Errorf("source = %q, want capture_health", first.Source)
	}
	if first.Kind == nil || *first.Kind != "reauth_required" {
		t.Errorf("kind = %v, want reauth_required", first.Kind)
	}
	// The mailbox the reader knows: the account label where one was
	// recorded, the provider where none was.
	if first.Detail == nil || *first.Detail != "rep@example.com" {
		t.Errorf("detail = %v, want the account label", first.Detail)
	}
	if first.Id != labelled.String() {
		t.Errorf("id = %q, want the connection id", first.Id)
	}
	second := (*out.CaptureHealth)[1]
	if second.Detail == nil || *second.Detail != "imap" {
		t.Errorf("unlabelled detail = %v, want the provider as fallback", second.Detail)
	}
	if len(first.Actions)+len(second.Actions) != 0 {
		t.Error("a capture card promises no verbs")
	}
}

func TestARefusedCaptureHealthReadIsNamedAsWithheld(t *testing.T) {
	svc := captureLaneService(&stubCaptureHealth{err: apperrors.ErrPermissionDenied})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.CaptureHealth != nil {
		t.Fatalf("capture_health = %v, want the lane withheld", out.CaptureHealth)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("capture_health")) {
		t.Fatal("a refused capture_health read was not named in lanes_omitted")
	}
}

// Healthy mailboxes are an EMPTY lane: the feed looked and found nothing
// broken, which the absent lane never promises.
func TestHealthyMailboxesReadAsAnEmptyLane(t *testing.T) {
	svc := captureLaneService(&stubCaptureHealth{})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.CaptureHealth == nil || len(*out.CaptureHealth) != 0 {
		t.Fatalf("capture_health = %v, want an empty lane", out.CaptureHealth)
	}
}
