// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reviewTemplateAdmin may read, add and change review templates, which the
// harness admin is not granted by default.
func reviewTemplateAdmin(e *Env) context.Context {
	perms := AdminPerms
	perms.Objects = maps.Clone(AdminPerms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	return e.As(e.AdminUser, nil, perms)
}

// keepReviewTemplates puts the template list back as the test found it. The
// list survives a workspace reset, so a template a test adds or retires would
// otherwise change what the next test is asked.
func keepReviewTemplates(t *testing.T) {
	t.Helper()
	type kept struct {
		id        ids.UUID
		active    bool
		questions []byte
	}
	rows, err := OwnerConn(t).Query(context.Background(), `SELECT id, active, questions FROM activity_review_template`)
	if err != nil {
		t.Fatalf("keeping the review templates: %v", err)
	}
	keep, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (kept, error) {
		var k kept
		return k, row.Scan(&k.id, &k.active, &k.questions)
	})
	if err != nil {
		t.Fatalf("keeping the review templates: %v", err)
	}
	t.Cleanup(func() {
		owner := OwnerConn(t)
		kept := make([]ids.UUID, len(keep))
		for i, k := range keep {
			kept[i] = k.id
		}
		// Every row goes inactive first, so restoring one active template per
		// outcome never meets a second one on the unique index.
		if _, err := owner.Exec(context.Background(), `DELETE FROM activity_review_template WHERE id <> ALL($1)`, kept); err != nil {
			t.Errorf("removing added review templates: %v", err)
		}
		if _, err := owner.Exec(context.Background(), `UPDATE activity_review_template SET active = false`); err != nil {
			t.Errorf("restoring the review templates: %v", err)
		}
		for _, k := range keep {
			if _, err := owner.Exec(context.Background(),
				`UPDATE activity_review_template SET active = $2, questions = $3 WHERE id = $1`, k.id, k.active, k.questions); err != nil {
				t.Errorf("restoring review template %s: %v", k.id, err)
			}
		}
	})
}

func liveTemplate(ctx context.Context, t *testing.T, e *Env, outcome string) crmcontracts.ActivityReviewTemplate {
	t.Helper()
	templates, err := e.Activities.ListReviewTemplates(ctx)
	if err != nil {
		t.Fatalf("listing review templates: %v", err)
	}
	for _, template := range templates {
		if string(template.Outcome) == outcome && template.Active {
			return template
		}
	}
	t.Fatalf("no active %s template", outcome)
	return crmcontracts.ActivityReviewTemplate{}
}

func reviewTemplateByID(ctx context.Context, t *testing.T, e *Env, id openapi_types.UUID) crmcontracts.ActivityReviewTemplate {
	t.Helper()
	templates, err := e.Activities.ListReviewTemplates(ctx)
	if err != nil {
		t.Fatalf("listing review templates: %v", err)
	}
	for _, template := range templates {
		if template.Id == id {
			return template
		}
	}
	t.Fatalf("template %s is no longer listed", id)
	return crmcontracts.ActivityReviewTemplate{}
}

func writeWinReview(ctx context.Context, e *Env, deal ids.DealID, closing ids.UUID, answers map[string]string) (crmcontracts.OutcomeReview, error) {
	return compose.NewOutcomeReviews(e.Pool).Write(ctx, deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(closing), SubmissionId: openapi_types.UUID(ids.NewV7()), Answers: answers,
	})
}

func TestAnAddedReviewTemplateIsAskedAndRetiresTheOneBefore(t *testing.T) {
	e := Setup(t)
	keepReviewTemplates(t)
	ctx := reviewTemplateAdmin(e)
	before := liveTemplate(ctx, t, e, "won")

	added, err := e.Activities.CreateReviewTemplate(ctx, crmcontracts.CreateActivityReviewTemplateRequest{
		Label: "Win review", Outcome: "won",
		Questions: []crmcontracts.ReviewQuestion{{Key: "champion", Label: "Who championed us?", Type: "text", Required: true}},
	})
	if err != nil {
		t.Fatalf("adding a template: %v", err)
	}
	// The shipped template holds win_review, so the derived key takes a number.
	if added.Key != "win_review_2" || !added.Active || added.System {
		t.Fatalf("added template = key %q active %v system %v", added.Key, added.Active, added.System)
	}
	if reviewTemplateByID(ctx, t, e, before.Id).Active {
		t.Fatal("the template asked before is still active, so an outcome asks two")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'create'`, added.Id); got != 1 {
		t.Fatalf("audit rows for the added template = %d, want 1", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type' = 'reporting.changed' AND envelope::text LIKE '%' || $1 || '%'`, added.Id.String()); got != 1 {
		t.Fatalf("reporting.changed events for the added template = %d, want 1", got)
	}

	deal, closing := closedDeal(t, e, "Asked the added template")
	review, err := writeWinReview(ctx, e, deal, closing, map[string]string{"champion": "The CFO"})
	if err != nil {
		t.Fatalf("writing a review on the added template: %v", err)
	}
	if review.TemplateKey != added.Key || len(review.Questions) != 1 || review.Questions[0].Key != "champion" {
		t.Fatalf("review froze template %q with %v, want the added one", review.TemplateKey, review.Questions)
	}
}

func TestARetiredReviewTemplateIsNotAskedAndItsReviewsStillRead(t *testing.T) {
	e := Setup(t)
	keepReviewTemplates(t)
	ctx := reviewTemplateAdmin(e)
	template := liveTemplate(ctx, t, e, "won")
	pipeline, open, won := DealFixture(t, e)
	deal, closing := closedDealIn(t, e, "Reviewed before retirement", pipeline, open, won)
	if _, err := writeWinReview(ctx, e, deal, closing, map[string]string{"why_we_won": "Fit"}); err != nil {
		t.Fatalf("writing the review before retirement: %v", err)
	}

	off := false
	retired, err := e.Activities.UpdateReviewTemplate(ctx, ids.UUID(template.Id),
		crmcontracts.UpdateActivityReviewTemplateRequest{Version: *template.Version, Active: &off})
	if err != nil {
		t.Fatalf("retiring the template: %v", err)
	}
	if retired.Active || len(retired.Questions) != len(template.Questions) {
		t.Fatalf("retiring changed more than active: %+v", retired)
	}
	if _, err := e.Activities.UpdateReviewTemplate(ctx, ids.UUID(template.Id),
		crmcontracts.UpdateActivityReviewTemplateRequest{Version: *template.Version, Active: &off}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a retirement on a stale version = %v, want a conflict", err)
	}

	// The last won template is retired, so a won closing asks no review.
	next, nextClosing := closedDealIn(t, e, "Closed after retirement", pipeline, open, won)
	if _, err := writeWinReview(ctx, e, next, nextClosing, map[string]string{"why_we_won": "Fit"}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a review on a retired template = %v, want not found", err)
	}
	old, err := compose.NewOutcomeReviews(e.Pool).List(ctx, deal)
	if err != nil || len(old) != 1 || old[0].TemplateKey != template.Key || len(old[0].Questions) != len(template.Questions) {
		t.Fatalf("the review written before retirement no longer reads: %v %v", old, err)
	}

	// Turning a retired template back on retires the one added meanwhile.
	added, err := e.Activities.CreateReviewTemplate(ctx, crmcontracts.CreateActivityReviewTemplateRequest{
		Label: "Short win review", Outcome: "won",
		Questions: []crmcontracts.ReviewQuestion{{Key: "why", Label: "Why?", Type: "text", Required: true}},
	})
	if err != nil {
		t.Fatalf("adding a template: %v", err)
	}
	on := true
	if _, err := e.Activities.UpdateReviewTemplate(ctx, ids.UUID(template.Id),
		crmcontracts.UpdateActivityReviewTemplateRequest{Version: *retired.Version, Active: &on}); err != nil {
		t.Fatalf("restoring the template: %v", err)
	}
	if reviewTemplateByID(ctx, t, e, added.Id).Active {
		t.Fatal("restoring a template left the one added meanwhile active")
	}
}

func TestOnlyAGrantedSeatAddsOrRetiresAReviewTemplate(t *testing.T) {
	e := Setup(t)
	keepReviewTemplates(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	template := liveTemplate(reviewTemplateAdmin(e), t, e, "lost")

	_, err := e.Activities.CreateReviewTemplate(rep, crmcontracts.CreateActivityReviewTemplateRequest{
		Label: "Rep loss review", Outcome: "lost",
		Questions: []crmcontracts.ReviewQuestion{{Key: "why", Label: "Why?", Type: "text", Required: true}},
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep adding a template = %v, want permission denied", err)
	}
	off := false
	_, err = e.Activities.UpdateReviewTemplate(rep, ids.UUID(template.Id),
		crmcontracts.UpdateActivityReviewTemplateRequest{Version: *template.Version, Active: &off})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep retiring a template = %v, want permission denied", err)
	}
	if !liveTemplate(reviewTemplateAdmin(e), t, e, "lost").Active {
		t.Fatal("a refused retirement retired the template")
	}
}
