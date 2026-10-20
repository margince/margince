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
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestProjectArchivePreviewMatchesTheRetiredScope(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			for _, object := range []string{"relationship", "tag", "list"} {
				perms.Objects[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
			}
			ctx := e.As(e.AdminUser, nil, perms)
			company := e.SeedCompany(t, "Project account", &e.AdminUser)
			project, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: "Archive this project", CompanyID: ids.From[ids.CompanyKind](company), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.From[ids.ProjectKind](ids.UUID(project.Id))
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal, err := deals.NewStore(e.DB(), DealsInstallation()).CreateDeal(ctx, deals.CreateDealInput{Name: "Keep project deal", PipelineID: pipeline, StageID: stage, ProjectID: &id, CompanyID: new(ids.From[ids.CompanyKind](company)), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Keep project history"), Links: []activities.ActivityLinkInput{{EntityType: "project", EntityID: id.UUID}}})
			if err != nil {
				t.Fatal(err)
			}
			contact := e.SeedContact(t, "Project sponsor", &e.AdminUser)
			edge, err := e.Contacts.SetProjectStakeholder(ctx, contacts.SetProjectStakeholderInput{ProjectID: id, ContactID: ids.From[ids.ContactKind](contact), Role: "sponsor"})
			if err != nil {
				t.Fatal(err)
			}
			collection := collections.NewStore(e.DB())
			list, err := collection.CreateList(ctx, collections.CreateListInput{Name: "Active projects", EntityType: "project"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := collection.AddMember(ctx, list.ID, collections.MemberChange{EntityType: "project", EntityID: id.UUID, Reason: collections.ReasonChosen}); err != nil {
				t.Fatal(err)
			}
			tag, err := collection.CreateTag(ctx, "Delivery", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := collection.ApplyTag(ctx, tag.ID, "project", id.UUID); err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"record_type":"project","id":%q}`, id)
			if rest {
				payload = fmt.Sprintf(`{"operation":"archiveProject","path":"/v1/projects/%s","body":null}`, id)
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			approval, err := svc.Stage(ctx, approvals.StageInput{Kind: "archive_record", TargetType: "project", TargetID: id.UUID, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			wire, err := svc.GetWire(ctx, approval)
			if err != nil {
				t.Fatal(err)
			}
			preview := wire.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil || len(*preview.Effects) != 1 || (*preview.Effects)[0] != "archive_project_detach_scope" {
				t.Fatalf("preview=%+v", preview)
			}
			fields := map[string]string{}
			for _, detail := range *preview.Details {
				fields[detail.Field] = detail.Value.Text
			}
			for field, want := range map[string]string{"archive_stakeholder": "Project sponsor", "archive_role": "sponsor", "archive_list": "Active projects", "archive_tag": "Delivery"} {
				if fields[field] != want {
					t.Fatalf("%s=%q want %q", field, fields[field], want)
				}
			}
			if wire.TargetVersion == nil {
				t.Fatal("missing target version")
			}
			if _, err := e.Projects.ArchiveProject(ctx, id, new(int64(*wire.TargetVersion))); err != nil {
				t.Fatal(err)
			}
			if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
				var retired bool
				if err := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM relationship WHERE id=@id`, pgx.StrictNamedArgs{"id": edge.ID}).Scan(&retired); err != nil {
					return err
				}
				if !retired {
					t.Error("stakeholder survived archive")
				}
				var memberships, tags, companyEdges int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM list_member WHERE entity_type='project' AND entity_id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&memberships); err != nil {
					return err
				}
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM taggable WHERE entity_type='project' AND entity_id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&tags); err != nil {
					return err
				}
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM relationship WHERE project_id=@id AND kind='project_company' AND archived_at IS NULL`, pgx.StrictNamedArgs{"id": id}).Scan(&companyEdges); err != nil {
					return err
				}
				if memberships != 0 || tags != 0 || companyEdges != 1 {
					t.Errorf("memberships=%d tags=%d company links=%d", memberships, tags, companyEdges)
				}
				var dealPreserved, activityPreserved bool
				if err := tx.QueryRow(ctx, `SELECT
                EXISTS (SELECT 1 FROM deal WHERE id=@deal AND project_id=@project AND archived_at IS NULL),
                EXISTS (SELECT 1 FROM activity a JOIN activity_link l ON l.activity_id=a.id WHERE a.id=@activity AND a.archived_at IS NULL AND l.project_id=@project)`, pgx.StrictNamedArgs{"deal": deal.Id, "project": id, "activity": activity.Id}).Scan(&dealPreserved, &activityPreserved); err != nil {
					return err
				}
				if !dealPreserved || !activityPreserved {
					t.Errorf("deal preserved=%t activity preserved=%t", dealPreserved, activityPreserved)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
