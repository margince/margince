// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMergePreviewNamesCarriedCommitmentsAndChecksPlanReader(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["weekly_plan"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
			ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
			source, target := mergeContentPair(t, e, kind)
			store := weeklyPlanStore(e.Pool)
			now := time.Date(2026, time.June, 10, 9, 0, 0, 0, time.UTC)
			if _, err := store.StartWeek(ctx, now); err != nil {
				t.Fatal(err)
			}
			commitment, err := store.AddCommitment(ctx, now, weeklyplan.NewCommitment{Label: "Review the proposal", LinkedRecordType: kind, LinkedRecordID: source})
			if err != nil {
				t.Fatal(err)
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Commitments) != 1 || ids.UUID(preview.Commitments[0].Id) != commitment.ID || preview.Commitments[0].Label != commitment.Label {
				t.Fatalf("commitments=%+v", preview.Commitments)
			}
			hidden, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.Rep3, []ids.UUID{e.Team2}, perms), kind, source, target, nil, nil)
			if !errors.Is(err, apperrors.ErrNotFound) || hidden.Commitments != nil {
				t.Fatalf("other reader preview=%+v error=%v", hidden, err)
			}
			if kind == "contact" {
				_, err = e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), &preview.SourceVersion)
			} else {
				_, err = e.Contacts.MergeCompany(ctx, ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target), &preview.SourceVersion)
			}
			if err != nil {
				t.Fatal(err)
			}
			plan, err := store.Current(ctx, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Commitments) != 1 || plan.Commitments[0].LinkedRecordID != target {
				t.Fatalf("carried commitments=%+v", plan.Commitments)
			}
		})
	}
}
