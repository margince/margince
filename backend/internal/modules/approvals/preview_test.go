// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlySupportedCompletePreviewVersionsCanBeApproved(t *testing.T) {
	for _, version := range []int{0, 1, 2, 3} {
		for _, state := range []string{"ready", "incomplete", "unavailable", "stale"} {
			for _, blocks := range []bool{false, true} {
				preview := &crmcontracts.ApprovalPreview{
					Version: version, State: state,
					Conflicts: &[]crmcontracts.ApprovalPreviewConflict{{Kind: "scope_changed", BlocksApproval: blocks}},
				}
				want := (version == 1 || version == 2) && state == "ready" && !blocks
				if allowed := PreviewRefusal(preview) == nil; allowed != want {
					t.Fatalf("version=%d state=%s blocks=%t allowed=%t", version, state, blocks, allowed)
				}
			}
		}
	}
}

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
			preview, _, err := svc.proposalPreview(context.Background(), row{Kind: kind}, crmcontracts.Approval{Kind: kind})
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
		preview, _, err := svc.proposalPreview(context.Background(), row{}, crmcontracts.Approval{})
		if err != nil || preview.State != state || len(preview.Changes) != 0 || preview.Effects != nil {
			t.Fatalf("unavailable comparison: %+v, %v", preview, err)
		}
	}
}

func TestStagePreviewPinMustNameTheProposedDestination(t *testing.T) {
	stage := ids.NewV7()
	subject := row{Kind: "advance_deal", CoTargetType: new("stage"), CoTargetID: &stage, CoTargetVersion: new(int64(1))}
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
			if got := previewCoTargetPinned(subject, crmcontracts.Approval{ProposedChange: &payload}); got != same {
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

func TestTagMergePreviewCannotRetargetItsPinnedSurvivor(t *testing.T) {
	survivor := ids.NewV7()
	subject := row{Kind: "merge_tags", CoTargetType: new("tag"), CoTargetID: &survivor, CoTargetVersion: new(int64(1))}
	for _, rest := range []bool{false, true} {
		for _, same := range []bool{false, true} {
			destination := ids.NewV7()
			if same {
				destination = survivor
			}
			payload := map[string]any{"into_tag_id": destination.String()}
			if rest {
				payload = map[string]any{"operation": "mergeTags", "body": payload}
			}
			if got := previewCoTargetPinned(subject, crmcontracts.Approval{ProposedChange: &payload}); got != same {
				t.Fatalf("REST=%v same=%v pinned=%v", rest, same, got)
			}
		}
	}
	subject.CoTargetVersion = nil
	payload := map[string]any{"into_tag_id": survivor.String()}
	if previewCoTargetPinned(subject, crmcontracts.Approval{ProposedChange: &payload}) {
		t.Fatal("legacy unpinned survivor treated as ready")
	}
}

func TestAnEditedTagMergeCannotSpendAnotherSurvivorsPin(t *testing.T) {
	survivor := ids.NewV7()
	subject := row{Kind: "merge_tags", CoTargetType: new("tag"), CoTargetID: &survivor, CoTargetVersion: new(int64(1))}
	service := NewService(nil).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
		return &crmcontracts.ApprovalPreview{Version: 1, State: "ready", Changes: []crmcontracts.ApprovalPreviewChange{}, Effects: &[]string{"merge_tags_release_source"}}, nil
	})
	for _, rest := range []bool{false, true} {
		payload := map[string]any{"into_tag_id": ids.NewV7().String()}
		if rest {
			payload = map[string]any{"operation": "mergeTags", "body": payload}
		}
		edited, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		var refusal *PreviewContextError
		if err := service.checkPreview(context.Background(), subject, edited); !errors.As(err, &refusal) || refusal.State != "incomplete" {
			t.Fatalf("edited survivor accepted: %v", err)
		}
	}
}

func TestProgressionPreviewRequiresItsOriginalDestinationPin(t *testing.T) {
	destination := ids.NewV7()
	subject := row{Kind: "stage_progression", CoTargetType: new("stage"), CoTargetID: &destination, CoTargetVersion: new(int64(1))}
	payload := map[string]any{"to_stage_id": destination.String()}
	proposal := crmcontracts.Approval{ProposedChange: &payload}
	if !previewCoTargetPinned(subject, proposal) {
		t.Fatal("matching destination refused")
	}
	payload["to_stage_id"] = ids.NewV7().String()
	if previewCoTargetPinned(subject, proposal) {
		t.Fatal("edited destination spent another pin")
	}
	payload["to_stage_id"] = destination.String()
	subject.CoTargetVersion = nil
	if previewCoTargetPinned(subject, proposal) {
		t.Fatal("legacy proposal without a stage pin appears ready")
	}
}

// stalePreviewWithAnUnnamedValue answers a stale preview showing one value as
// unavailable_name, and tells the recorder it masked it when masked is true.
func stalePreviewWithAnUnnamedValue(masked bool) func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
	return func(ctx context.Context, _ crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
		if masked {
			RecordMasked(ctx)
		}
		details := []crmcontracts.ApprovalPreviewDetail{{Field: "to", Value: crmcontracts.ApprovalPreviewValue{Kind: "unavailable_name"}}}
		return &crmcontracts.ApprovalPreview{Version: 2, State: "stale", Changes: []crmcontracts.ApprovalPreviewChange{}, Details: &details}, nil
	}
}

func stagedEmailRow() []crmcontracts.Approval {
	change := map[string]any{"to": []any{"buyer@masked.test"}}
	summary := "Write to buyer@masked.test"
	parts := &crmcontracts.ApprovalSummaryParts{Sentence: "Send email", Fields: []crmcontracts.ApprovalSummaryField{{Field: "to", Value: "buyer@example.test"}}}
	return []crmcontracts.Approval{{Kind: "send_email", Status: crmcontracts.ApprovalStatusPending, ProposedChange: &change, Summary: &summary, SummaryParts: parts}}
}

// A preview that is not ready is cleared before it leaves the service. A value
// it masked must still keep the staged call back.
func TestAMaskedValueKeepsTheStagedCallBackAfterThePreviewIsCleared(t *testing.T) {
	svc := NewService(nil).WithPreview(stalePreviewWithAnUnnamedValue(true))
	rows := stagedEmailRow()
	if err := svc.readDecisionContext(context.Background(), rows, []row{{Kind: "send_email"}}); err != nil {
		t.Fatal(err)
	}
	if rows[0].Preview == nil || rows[0].Preview.Details != nil {
		t.Fatalf("the stale preview was not cleared: %+v", rows[0].Preview)
	}
	if rows[0].ProposedChange != nil || rows[0].Summary != nil || rows[0].SummaryParts != nil {
		t.Fatalf("the staged call kept a masked value: %+v %v %+v", rows[0].ProposedChange, rows[0].Summary, rows[0].SummaryParts)
	}
}

// An archived seat shows as unavailable_name with nothing masked. The staged
// call of a stale proposal its decider runs then stays on the approval.
func TestAnUnnamedValueThatNoMaskHidKeepsTheStagedCall(t *testing.T) {
	svc := NewService(nil).WithPreview(stalePreviewWithAnUnnamedValue(false))
	rows := stagedEmailRow()
	if err := svc.readDecisionContext(context.Background(), rows, []row{{Kind: "send_email"}}); err != nil {
		t.Fatal(err)
	}
	if rows[0].ProposedChange == nil || rows[0].Summary == nil {
		t.Fatal("an unnamed value with no mask took the staged call away")
	}
}

// A committed verdict whose context could not be read gets an unavailable
// preview, and nothing of its plan beside it.
func TestACommittedVerdictWithoutContextCarriesNoPlan(t *testing.T) {
	svc := NewService(nil).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
		return nil, errors.New("the preview reader is down")
	})
	summary := "Write to the buyer"
	subject := row{
		Kind: "send_email", Status: "pending", ProposedChange: json.RawMessage(`{"to":["buyer@example.test"]}`), Summary: &summary,
		SummaryParts: json.RawMessage(`{"sentence":"Write to the buyer","fields":[{"field":"to","value":"buyer@example.test"}],"more_fields":0}`),
	}
	rows := []crmcontracts.Approval{wire(subject, svc.now())}
	if rows[0].SummaryParts == nil {
		t.Fatal("the fixture's parts did not read back, so the check below proves nothing")
	}
	svc.readCommittedContext(context.Background(), rows, []row{subject})
	if rows[0].Preview == nil || rows[0].Preview.State != "unavailable" {
		t.Fatalf("the fallback preview = %+v", rows[0].Preview)
	}
	if rows[0].ProposedChange != nil || rows[0].Summary != nil || rows[0].Evidence != nil || rows[0].SummaryParts != nil {
		t.Fatalf("the fallback still carries the plan: %+v %v %v", rows[0].ProposedChange, rows[0].Summary, rows[0].Evidence)
	}
}
