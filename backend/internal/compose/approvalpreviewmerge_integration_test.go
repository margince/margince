// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestRecordMergePreviewUsesTheNativeSurvivorshipPlan(t *testing.T) {
	e := integration.Setup(t)
	permissions := integration.AdminPerms
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects["introduction"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.AdminUser, nil, permissions)
	source, survivor := mergeContentPair(t, e, "contact")
	records := NewProviderFor(e.DB())
	target, err := records.Read(ctx, datasource.EntityRef{Type: datasource.EntityContact, ID: survivor})
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"record_type": "contact", "source_id": source.String(), "target_id": survivor.String()}
	proposal := crmcontracts.Approval{Kind: "merge_records", Status: crmcontracts.ApprovalStatusPending, TargetEntityType: new("contact"), TargetEntityId: new(openapi_types.UUID(survivor)), TargetVersion: new(int(target.Version)), ProposedChange: &payload}
	reader := newApprovalPreviewReader(e.DB())
	scope, scopeErr := records.PreviewMergeSurvivorship(ctx, "contact", source, survivor, nil, &target.Version)
	if scopeErr != nil {
		t.Fatalf("native scope: %v", scopeErr)
	}
	if err := reader.populateMerge(ctx, recordMergePreviewRequest{RecordType: "contact", SourceID: source, TargetID: survivor}, scope, &crmcontracts.ApprovalPreview{}); err != nil {
		t.Fatalf("merge display: %v", err)
	}
	preview, err := reader.recordMergePreview(ctx, proposal, &crmcontracts.ApprovalPreview{Changes: []crmcontracts.ApprovalPreviewChange{}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.State != previewReadyState || preview.Details == nil || len(*preview.Details) < 2 || preview.Effects == nil {
		t.Fatalf("incomplete merge preview: %+v", preview)
	}
	details := *preview.Details
	if details[0].Value.EntityId == nil || *details[0].Value.EntityId != openapi_types.UUID(source) || details[1].Value.EntityId == nil || *details[1].Value.EntityId != openapi_types.UUID(survivor) {
		t.Fatalf("wrong merge pair: %+v", details[:2])
	}
	if details[0].Value.Text == "" || details[1].Value.Text == "" || (*preview.Effects)[0] != "merge_records_release_source" {
		t.Fatalf("missing named effect: %+v", preview)
	}
	proposal.TargetVersion = new(int(target.Version + 1))
	stale, err := reader.recordMergePreview(ctx, proposal, &crmcontracts.ApprovalPreview{Changes: []crmcontracts.ApprovalPreviewChange{}})
	if err != nil || stale.State != previewStaleState || stale.Details != nil {
		t.Fatalf("changed survivor disclosed ready scope: preview=%+v error=%v", stale, err)
	}
	result, err := records.Merge(ctx, datasource.MergeInput{Type: datasource.EntityContact, SourceID: source, TargetID: survivor})
	if err != nil || result.ID != survivor {
		t.Fatalf("native merge=%+v error=%v", result, err)
	}
	proposal.TargetVersion = new(int(target.Version))
	retired, err := reader.recordMergePreview(ctx, proposal, &crmcontracts.ApprovalPreview{Changes: []crmcontracts.ApprovalPreviewChange{}})
	if err != nil || retired.State == previewReadyState || retired.Details != nil {
		t.Fatalf("retired source still reviewable: preview=%+v error=%v", retired, err)
	}
}
