// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
)

func TestEveryGovernedKindHasReviewContextOrIsNotCurrentlyStageable(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	active := map[string]bool{}
	for _, spec := range registry.Specs() {
		if registry.Stageable(spec.Name) {
			active[spec.Name] = true
		}
	}
	for _, kind := range automation.StageableKinds() {
		active[kind] = true
	}
	for _, kind := range approvalsServiceWithEffects(nil).EffectKinds() {
		active[kind] = true
	}
	for _, policy := range agentPolicies {
		if policy.Access == accessTool && policy.Tier != tierAutoExecute {
			active[policy.Tool] = true
		}
	}
	governed := approvals.StageableKinds()
	if len(governed) == 0 || len(active) == 0 {
		t.Fatal("the approval producer census is empty")
	}
	reader := approvalPreviewReader{}
	for _, kind := range governed {
		if inactiveApprovalKind(kind) {
			if active[kind] {
				t.Errorf("%s became stageable without a display", kind)
			}
			delete(active, kind)
			continue
		}
		assertApprovalContextCovered(t, reader, kind)
		delete(active, kind)
	}
	for kind := range active {
		t.Errorf("active producer %s is missing from the governed approval catalog", kind)
	}
}

func inactiveApprovalKind(kind string) bool {
	// The first two execute without staging; the two retired kinds remain readable in history.
	switch kind {
	case "archive_tag", "change_record_parts", "transcript_proposal", "communication_review":
		return true
	}
	return false
}

func assertApprovalContextCovered(t *testing.T, reader approvalPreviewReader, kind string) {
	t.Helper()
	preview, err := reader.read(context.Background(), crmcontracts.Approval{Kind: kind, Status: crmcontracts.ApprovalStatusPending})
	if err != nil {
		t.Fatalf("%s missing-context projection: %v", kind, err)
	}
	if hasTypedProposalDisplay(kind) {
		if preview != nil {
			t.Errorf("%s replaced its established typed display", kind)
		}
		return
	}
	if preview == nil {
		t.Fatalf("%s falls back to raw arguments", kind)
	}
	if preview.Conflicts != nil {
		for _, conflict := range *preview.Conflicts {
			if conflict.Kind == "unsupported_approval_kind" {
				t.Errorf("%s has no display projection", kind)
			}
		}
	}
	if err := approvals.PreviewRefusal(preview); err == nil {
		t.Errorf("%s permits approval without required context", kind)
	}
}

// An extension's staged kind is its tool name, registered at boot from the
// served tools; the census walks that same derivation.
func TestEveryStageableExtensionVerbHasReviewContext(t *testing.T) {
	kinds := extensionStagingKinds(confirmFirstUnit(t))
	if len(kinds) == 0 {
		t.Fatal("the confirm-first unit derived no staged kind")
	}
	if err := approvals.RegisterExtensionKinds(kinds); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := approvals.RegisterExtensionKinds(nil); err != nil {
			t.Errorf("clearing the registered kinds: %v", err)
		}
	})
	for _, kind := range kinds {
		assertApprovalContextCovered(t, approvalPreviewReader{}, kind.Verb)
	}
}

func TestAnUnknownApprovalKindRequiresSupportedContext(t *testing.T) {
	preview, err := (approvalPreviewReader{}).read(context.Background(), crmcontracts.Approval{Kind: "new_unsupported_operation", Status: crmcontracts.ApprovalStatusPending})
	if err != nil {
		t.Fatal(err)
	}
	if preview == nil || approvals.PreviewRefusal(preview) == nil {
		t.Fatal("unknown effect can be approved without context")
	}
}
