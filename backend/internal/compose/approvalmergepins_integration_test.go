// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestMergeRechecksBothApprovedRecordsInsideTheNativeWrite(t *testing.T) {
	for _, kind := range []datasource.EntityType{datasource.EntityContact, datasource.EntityCompany} {
		for _, changed := range []string{"neither", "source", "survivor"} {
			t.Run(string(kind)+"/"+changed, func(t *testing.T) {
				e := integration.Setup(t)
				permissions := integration.AdminPerms
				permissions.Objects = maps.Clone(permissions.Objects)
				// The seeded admin reads every record class a company merge moves; this fixture lacks these.
				for _, object := range []string{"introduction", mergeReadOffer, mergeReadCommission, mergeReadFinance, mergeReadIntegrations} {
					permissions.Objects[object] = principal.ObjectGrant{Read: true}
				}
				ctx := e.As(e.AdminUser, nil, permissions)
				native := NewProvider(e.Pool)
				field := "full_name"
				if kind == datasource.EntityCompany {
					field = "display_name"
				}
				create := func(name string) datasource.EntityRef {
					t.Helper()
					ref, err := native.Create(ctx, datasource.CreateInput{EntityType: kind, Fields: json.RawMessage(fmt.Sprintf(`{%q:%q}`, field, name)), Source: "manual"})
					if err != nil {
						t.Fatal(err)
					}
					return ref
				}
				source, survivor := create("Source record"), create("Surviving record")
				rename := func(ref datasource.EntityRef, name string) {
					t.Helper()
					if _, err := native.Update(ctx, datasource.UpdateInput{Ref: ref, Patch: json.RawMessage(fmt.Sprintf(`{%q:%q}`, field, name)), Source: "manual"}); err != nil {
						t.Fatal(err)
					}
				}
				rename(survivor, "Reviewed survivor")
				info, err := agents.StageSubject(ctx, agents.NewMergeCall(native, nil, agents.MergeCommand{RecordType: string(kind), SourceID: source.ID, TargetID: survivor.ID}))
				if err != nil {
					t.Fatal(err)
				}
				raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":%q,"source_id":%q,"target_id":%q}`, kind, source.ID, survivor.ID)))
				if err != nil {
					t.Fatal(err)
				}
				svc := NewApprovalsService(e.DB())
				approval, err := svc.Stage(ctx, approvals.StageInput{Kind: "merge_records", ProposedChange: raw, DiffHash: hash, TargetType: info.TargetType, TargetID: info.TargetID, CoTargetType: info.CoTargetType, CoTargetID: info.CoTargetID, Summary: info.Summary})
				if err != nil {
					t.Fatal(err)
				}
				scope, err := native.PreviewMergeSurvivorship(ctx, string(kind), source.ID, survivor.ID, nil, nil)
				if err != nil {
					t.Fatalf("native merge scope unavailable: %v", err)
				}
				reader := newApprovalPreviewReader(e.DB())
				if err := reader.populateMerge(ctx, recordMergePreviewRequest{RecordType: string(kind), SourceID: source.ID, TargetID: survivor.ID}, scope, &crmcontracts.ApprovalPreview{}); err != nil {
					t.Fatalf("merge scope cannot be displayed: %v; fields=%+v", err, scope.Fields)
				}
				if _, err := svc.Decide(ctx, approval, true, nil); err != nil {
					t.Fatal(err)
				}
				released, _, _, err := agents.RedeemAndMark(ctx, approvalsAdapter{svc: svc}, approval, "merge_records", hash)
				if err != nil {
					t.Fatal(err)
				}
				if changed == "source" {
					rename(source, "Source changed after redemption")
				}
				if changed == "survivor" {
					rename(survivor, "Survivor changed after redemption")
				}
				result, err := native.Merge(released, datasource.MergeInput{Type: kind, SourceID: source.ID, TargetID: survivor.ID})
				if changed == "neither" {
					if err != nil || result != survivor {
						t.Fatalf("merge=%+v err=%v", result, err)
					}
					return
				}
				if !errors.Is(err, apperrors.ErrVersionSkew) {
					t.Fatalf("changed %s merge=%+v err=%v", changed, result, err)
				}
				if _, err := native.Read(ctx, source); err != nil {
					t.Fatalf("refused merge removed source: %v", err)
				}
			})
		}
	}
}
