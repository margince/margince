// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRelinkPreviewExplainsPersistentProjectRetention(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	company := e.SeedCompany(t, "Delivery account", nil)
	project, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: "Delivery", CompanyID: companyIDOf(company), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Delivery discussion"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(activity.Id)
	in := activities.RelinkActivityInput{EntityType: "project", EntityID: ids.UUID(project.Id)}
	plan, err := e.Activities.PreviewRelink(ctx, []ids.UUID{id}, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || !plan[0].StampsCorrespondence {
		t.Fatalf("missing retention effect: %+v", plan)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_retention_evidence WHERE activity_id=$1`, id); n != 0 {
		t.Fatalf("preview wrote %d retention proofs", n)
	}
	if _, err := e.Activities.RelinkActivity(ctx, ids.From[ids.ActivityKind](id), in); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_retention_evidence WHERE activity_id=$1 AND project_id=$2`, id, project.Id); n != 1 {
		t.Fatalf("writer produced %d retention proofs", n)
	}
	plan, err = e.Activities.PreviewRelink(ctx, []ids.UUID{id}, in)
	if err != nil {
		t.Fatal(err)
	}
	if plan[0].StampsCorrespondence || plan[0].AddsLink {
		t.Fatalf("no-op promises another stamp: %+v", plan)
	}
	second, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: "Revised delivery", CompanyID: companyIDOf(company), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	in.EntityID, in.ReplaceExistingOfType = ids.UUID(second.Id), true
	plan, err = e.Activities.PreviewRelink(ctx, []ids.UUID{id}, in)
	if err != nil {
		t.Fatal(err)
	}
	if !plan[0].StampsCorrespondence || len(plan[0].Removed) != 1 || plan[0].Removed[0] != ids.UUID(project.Id) {
		t.Fatalf("replacement scope: %+v", plan)
	}
	if _, err := e.Activities.RelinkActivity(ctx, ids.From[ids.ActivityKind](id), in); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_retention_evidence WHERE activity_id=$1`, id); n != 2 {
		t.Fatalf("moving lost the old retention basis: %d", n)
	}
	missing := ids.NewV7()
	partial, err := e.Activities.PreviewRelink(ctx, []ids.UUID{id, missing}, in)
	if !errors.Is(err, apperrors.ErrNotFound) || partial != nil {
		t.Fatalf("failed batch disclosed partial scope: %+v, %v", partial, err)
	}
}
