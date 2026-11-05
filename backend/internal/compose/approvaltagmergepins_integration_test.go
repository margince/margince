// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTagMergeToolRechecksBothReleasedVersions(t *testing.T) {
	for _, changed := range []string{"neither", "source", "survivor"} {
		t.Run(changed, func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["tag"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
			ctx := e.As(e.AdminUser, nil, perms)
			store := collections.NewStore(e.DB())
			source, err := store.CreateTag(ctx, "Source category", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			target, err := store.CreateTag(ctx, "Surviving category", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			adapter := tagAdapter{store: store}
			info, err := agents.StageSubject(ctx, agents.NewMergeTagsCall(adapter, nil, agents.MergeTagsCommand{SourceID: source.ID.UUID, TargetID: target.ID.UUID}))
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(map[string]ids.UUID{"tag_id": source.ID.UUID, "into_tag_id": target.ID.UUID})
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			approval, err := svc.Stage(ctx, approvals.StageInput{Kind: "merge_tags", ProposedChange: raw, DiffHash: hash, TargetType: info.TargetType, TargetID: info.TargetID, CoTargetType: info.CoTargetType, CoTargetID: info.CoTargetID, Summary: info.Summary})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Decide(ctx, approval, true, nil); err != nil {
				t.Fatal(err)
			}
			released, _, _, err := agents.RedeemAndMark(ctx, approvalsAdapter{svc: svc}, approval, "merge_tags", hash)
			if err != nil {
				t.Fatal(err)
			}
			if changed != "neither" {
				id, version := source.ID, source.Version
				if changed == "survivor" {
					id, version = target.ID, target.Version
				}
				if _, err := store.UpdateTag(ctx, id, collections.TagUpdate{Name: new("Changed after redemption")}, version); err != nil {
					t.Fatal(err)
				}
			}
			_, err = adapter.MergeTags(released, source.ID.UUID, target.ID.UUID)
			if changed == "neither" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("changed %s accepted: %v", changed, err)
			}
			sourceAfter, _, err := store.GetTag(ctx, source.ID)
			if err != nil || sourceAfter.ArchivedAt != nil {
				t.Fatalf("refused merge retired source: %+v / %v", sourceAfter, err)
			}
		})
	}
}
