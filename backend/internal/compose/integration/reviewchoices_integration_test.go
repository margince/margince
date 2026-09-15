// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"maps"
	"reflect"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewChoicesRemainFrozenAfterTemplateEdits(t *testing.T) {
	e := Setup(t)
	perms := AdminPerms
	perms.Objects = maps.Clone(AdminPerms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true, Update: true}
	ctx := e.As(e.AdminUser, nil, perms)
	templates, err := e.Activities.ListReviewTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var template crmcontracts.ActivityReviewTemplate
	for _, candidate := range templates {
		if candidate.Outcome == "won" {
			template = candidate
		}
	}
	options := []string{"Fit, scope", "Trust"}
	question := crmcontracts.ReviewQuestion{Key: "reasons", Label: "Reasons", Type: "multiselect", Required: true, Options: &options}
	req := crmcontracts.UpdateActivityReviewTemplateRequest{Version: *template.Version, Questions: append(template.Questions, question)}
	updated, err := e.Activities.UpdateReviewTemplate(ctx, ids.UUID(template.Id), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.UpdateReviewTemplate(ctx, ids.UUID(template.Id), req); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale edit accepted: %v", err)
	}
	deal, closing := closedDeal(t, e, "Choice review")
	reviews := compose.NewOutcomeReviews(e.Pool)
	selected := map[string][]string{"reasons": {"Fit, scope", "Trust"}}
	review, err := reviews.Write(ctx, deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(closing), SubmissionId: openapi_types.UUID(ids.NewV7()),
		Answers: map[string]string{"why_we_won": "A good match"}, ChoiceAnswers: &selected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(review.ChoiceAnswers, &selected) {
		t.Fatalf("lost selected values: %v", review.ChoiceAnswers)
	}
	replacement := []string{"Other"}
	updated.Questions[len(updated.Questions)-1].Options = &replacement
	if _, err := e.Activities.UpdateReviewTemplate(ctx, ids.UUID(template.Id), crmcontracts.UpdateActivityReviewTemplateRequest{Version: *updated.Version, Questions: updated.Questions}); err != nil {
		t.Fatal(err)
	}

	_, staleErr := reviews.Write(ctx, deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(closing), SubmissionId: openapi_types.UUID(ids.NewV7()),
		Answers: map[string]string{"why_we_won": "Old form"}, TemplateVersion: updated.Version,
	})
	if !errors.Is(staleErr, apperrors.ErrConflict) {
		t.Fatalf("stale template accepted: %v", staleErr)
	}
	saved, err := reviews.List(ctx, deal)
	if err != nil || len(saved) != 1 {
		t.Fatalf("read: %v %v", saved, err)
	}
	frozen := saved[0].Questions[len(saved[0].Questions)-1]
	if !reflect.DeepEqual(frozen.Options, &options) || !reflect.DeepEqual(saved[0].ChoiceAnswers, &selected) {
		t.Fatal("template edit rewrote historical review")
	}
}
