// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAConflictCanRefuseAnOtherwiseReadyPreview(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		svc := NewService(nil).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
			return &crmcontracts.ApprovalPreview{Version: 1, State: "ready", Conflicts: &[]crmcontracts.ApprovalPreviewConflict{{Kind: "active_engagement", Summary: "Support continues", BlocksApproval: blocked}}}, nil
		})
		svc.now = func() time.Time { return time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC) }
		err := svc.checkPreview(context.Background(), row{}, nil)
		var refused *PreviewContextError
		if blocked && !errors.As(err, &refused) {
			t.Fatalf("blocking conflict: %v", err)
		}
		if !blocked && err != nil {
			t.Fatalf("informational conflict: %v", err)
		}
	}
}

func TestMissingPreviewContextIsNotReportedAsAChangedRecord(t *testing.T) {
	for _, state := range []string{"stale", "incomplete", "unavailable"} {
		svc := NewService(nil).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
			return &crmcontracts.ApprovalPreview{Version: 1, State: state}, nil
		})
		err := svc.checkPreview(context.Background(), row{}, nil)
		if state == "stale" {
			if !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("stale context: %v", err)
			}
			continue
		}
		var refused *PreviewContextError
		if !errors.As(err, &refused) || errors.Is(err, apperrors.ErrVersionSkew) {
			t.Fatalf("missing context reported as a changed record: %v", err)
		}
		code, message := refused.MessageFault()
		if code != "approval_preview_"+state || message == "" {
			t.Fatalf("context refusal: %q, %q", code, message)
		}
	}
}

func TestLegacyStageProposalsCannotPresentUnpinnedEffectsAsReady(t *testing.T) {
	for _, kind := range []string{"advance_deal", "progress_deal"} {
		t.Run(kind, func(t *testing.T) {
			svc := NewService(nil).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
				return &crmcontracts.ApprovalPreview{Version: 1, State: "ready", Changes: []crmcontracts.ApprovalPreviewChange{{Field: "stage_id"}}}, nil
			})
			preview, err := svc.proposalPreview(context.Background(), row{Kind: kind}, crmcontracts.Approval{Kind: kind})
			if err != nil || preview.State != "incomplete" || len(preview.Changes) != 0 {
				t.Fatalf("legacy preview: %+v, %v", preview, err)
			}
		})
	}
}

func TestUnavailablePreviewDoesNotPublishAnExecutableComparison(t *testing.T) {
	for _, state := range []string{"stale", "unavailable", "incomplete"} {
		svc := NewService(nil).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
			return &crmcontracts.ApprovalPreview{
				Version: 1, State: state,
				Changes: []crmcontracts.ApprovalPreviewChange{{Field: "lifecycle"}},
				Effects: &[]string{"start_project_delivery"},
			}, nil
		})
		preview, err := svc.proposalPreview(context.Background(), row{}, crmcontracts.Approval{})
		if err != nil || preview.State != state || len(preview.Changes) != 0 || preview.Effects != nil {
			t.Fatalf("unavailable comparison: %+v, %v", preview, err)
		}
	}
}

func TestStagePreviewPinMustNameTheProposedDestination(t *testing.T) {
	stage := ids.NewV7()
	subject := row{CoTargetType: new("stage"), CoTargetID: &stage, CoTargetVersion: new(int64(1))}
	for _, rest := range []bool{false, true} {
		for _, same := range []bool{false, true} {
			destination := ids.NewV7()
			if same {
				destination = stage
			}
			payload := map[string]any{"to_stage_id": destination.String()}
			if rest {
				payload = map[string]any{"operation": "advanceDeal", "body": payload}
			}
			if got := stagePreviewPinned(subject, crmcontracts.Approval{ProposedChange: &payload}); got != same {
				t.Fatalf("REST=%v same destination=%v pinned=%v", rest, same, got)
			}
		}
	}
}

func TestCommittedVerdictsSurviveContextOutagesWithoutEnablingPendingMembers(t *testing.T) {
	outage := errors.New("preview reader unavailable")
	svc := NewService(nil).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
		return nil, outage
	})
	svc.now = func() time.Time { return time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC) }
	for _, status := range []string{"approved", "rejected", "pending"} {
		subject := row{Status: status, ExpiresAt: svc.now().Add(time.Hour)}
		if _, err := svc.contextWire(context.Background(), subject); !errors.Is(err, outage) {
			t.Fatalf("read did not propagate context outage: %v", err)
		}
		got := svc.committedWire(context.Background(), subject)
		if string(got.Status) != status || got.Preview == nil || got.Preview.State != "unavailable" {
			t.Fatalf("committed status lost or context presented as usable: %+v", got)
		}
		if err := PreviewRefusal(got.Preview); err == nil {
			t.Fatal("missing context permits another decision")
		}
	}
}
