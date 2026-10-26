// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"maps"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A built-in question keeps its catalog key through a save that leaves its
// wording alone and loses it once reworded. A later review freezes the key.
func TestAReviewQuestionKeepsItsCatalogKeyUntilItIsReworded(t *testing.T) {
	e := Setup(t)
	perms := AdminPerms
	perms.Objects = maps.Clone(AdminPerms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true, Update: true}
	ctx := e.As(e.AdminUser, nil, perms)
	template := wonReviewTemplate(ctx, t, e)
	restoreReviewTemplate(t, e, template.Id)

	if template.LabelKey == nil || *template.LabelKey != "reviewTemplates.builtin.win_review" {
		t.Fatalf("the built-in win review names key %v", template.LabelKey)
	}
	keyOf := func(questions []crmcontracts.ReviewQuestion, key string) *string {
		for _, q := range questions {
			if q.Key == key {
				return q.LabelKey
			}
		}
		t.Fatalf("the template lost its %s question", key)
		return nil
	}
	if key := keyOf(template.Questions, "why_we_won"); key == nil || *key != "reviewTemplates.builtin.question.why_we_won" {
		t.Fatalf("why_we_won names key %v", key)
	}

	// Only the shipped questions: another test in this package adds its own.
	var edited []crmcontracts.ReviewQuestion
	for _, q := range template.Questions {
		if q.Key == "competitors" {
			q.Label = "Which other vendors were on the shortlist?"
		}
		if q.LabelKey != nil {
			edited = append(edited, q)
		}
	}
	updated, err := e.Activities.UpdateReviewTemplate(ctx, ids.UUID(template.Id),
		crmcontracts.UpdateActivityReviewTemplateRequest{Version: *template.Version, Questions: edited})
	if err != nil {
		t.Fatal(err)
	}
	if keyOf(updated.Questions, "why_we_won") == nil {
		t.Error("a question the save left alone lost its catalog key")
	}
	if key := keyOf(updated.Questions, "competitors"); key != nil {
		t.Errorf("a reworded question kept key %q, so the administrator's words never show", *key)
	}

	deal, closing := closedDeal(t, e, "Keyed review")
	review, err := compose.NewOutcomeReviews(e.Pool).Write(ctx, deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(closing), SubmissionId: openapi_types.UUID(ids.NewV7()),
		Answers: map[string]string{"why_we_won": "Fit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if keyOf(review.Questions, "why_we_won") == nil {
		t.Error("the frozen question dropped its catalog key, so the review reads in English")
	}
}

func wonReviewTemplate(ctx context.Context, t *testing.T, e *Env) crmcontracts.ActivityReviewTemplate {
	t.Helper()
	templates, err := e.Activities.ListReviewTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range templates {
		if candidate.Outcome == "won" && candidate.Key == "win_review" {
			return candidate
		}
	}
	t.Fatal("the shipped win review is gone")
	return crmcontracts.ActivityReviewTemplate{}
}

// The template table outlives the reset, so the seeded questions are put back.
func restoreReviewTemplate(t *testing.T, e *Env, id openapi_types.UUID) {
	t.Helper()
	var questions []byte
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT questions FROM activity_review_template WHERE id = $1`, id).Scan(&questions); err != nil {
		t.Fatalf("reading the seeded questions: %v", err)
	}
	owner := OwnerConn(t)
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(),
			`UPDATE activity_review_template SET questions = $2 WHERE id = $1`, id, questions); err != nil {
			t.Errorf("restoring the seeded questions: %v", err)
		}
	})
}
