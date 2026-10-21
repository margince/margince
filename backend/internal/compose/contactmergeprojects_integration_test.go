// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactMergeKeepsStakeholdersOnDifferentProjects(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["relationship"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	company := e.SeedCompany(t, "Project account", &e.AdminUser)
	source := ids.From[ids.ContactKind](e.SeedContact(t, "Source stakeholder", &e.AdminUser))
	target := ids.From[ids.ContactKind](e.SeedContact(t, "Surviving stakeholder", &e.AdminUser))
	projectIDs := []ids.UUID{}
	for i, contactIDs := range [][]ids.ContactID{{source}, {target}, {source, target}} {
		project, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: fmt.Sprintf("Project %d", i), CompanyID: ids.From[ids.CompanyKind](company), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		projectID := ids.From[ids.ProjectKind](ids.UUID(project.Id))
		projectIDs = append(projectIDs, projectID.UUID)
		for _, contact := range contactIDs {
			if _, err := e.Contacts.SetProjectStakeholder(ctx, contacts.SetProjectStakeholderInput{ProjectID: projectID, ContactID: contact, Role: "sponsor"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", source.UUID, target.UUID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Relationships) != 2 {
		t.Fatalf("relationships=%+v", preview.Relationships)
	}
	for _, edge := range preview.Relationships {
		if edge.Relationship.ProjectId == nil {
			t.Fatal("missing project")
		}
		if edge.Retired != (ids.UUID(*edge.Relationship.ProjectId) == projectIDs[2]) {
			t.Fatalf("wrong project disposition: %+v", edge)
		}
	}
	if _, err := e.Contacts.MergeContact(ctx, source, target, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		for _, project := range projectIDs {
			var live int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM relationship WHERE kind='project_stakeholder' AND project_id=@project AND contact_id=@target AND archived_at IS NULL`, pgx.StrictNamedArgs{"project": project, "target": target}).Scan(&live); err != nil {
				return err
			}
			if live != 1 {
				t.Errorf("project %s has %d surviving stakeholder links, want one", project, live)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
