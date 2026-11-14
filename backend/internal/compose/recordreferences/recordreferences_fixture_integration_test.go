// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordreferences_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type referenceFixture struct {
	id      ids.UUID
	read    func(context.Context, []ids.UUID) ([]crmcontracts.RecordReference, error)
	labels  func(context.Context, []ids.UUID) (map[ids.UUID]string, error)
	detail  func(context.Context) (crmcontracts.RecordReference, error)
	archive func() error
}

func makeReferenceFixture(t *testing.T, e *integration.Env, kind, name string) referenceFixture {
	t.Helper()
	ctx := e.Admin()
	switch kind {
	case "contact":
		row, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.ContactKind](ids.UUID(row.Id))
		return referenceFixture{
			id.UUID, e.Contacts.ContactReferences, e.Contacts.ContactLabels,
			func(ctx context.Context) (crmcontracts.RecordReference, error) {
				row, err := e.Contacts.GetContact(ctx, id, storekit.IncludeArchived)
				return crmcontracts.RecordReference{Id: row.Id, Name: &row.FullName, Archived: row.ArchivedAt != nil}, err
			}, func() error {
				_, err := e.Contacts.ArchiveContact(ctx, id, nil)
				return err
			},
		}
	case "company":
		row, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(row.Id))
		if _, _, err := e.Contacts.SetCompanyLogo(ctx, id, "reference-logo-revision", "https://example.test/logo.png"); err != nil {
			t.Fatal(err)
		}
		return referenceFixture{
			id.UUID, e.Contacts.CompanyReferences, e.Contacts.CompanyLabels,
			func(ctx context.Context) (crmcontracts.RecordReference, error) {
				row, err := e.Contacts.GetCompany(ctx, id, storekit.IncludeArchived)
				return crmcontracts.RecordReference{Id: row.Id, Name: &row.DisplayName, Archived: row.ArchivedAt != nil, Logo: row.LogoUrl}, err
			}, func() error {
				_, err := e.Contacts.ArchiveCompany(ctx, id, nil)
				return err
			},
		}
	case "lead":
		email := ids.NewV7().String() + "@example.test"
		row, created, err := e.Contacts.CreateLead(ctx, contacts.CreateLeadInput{FullName: &name, Email: &email, Source: "manual"})
		if err != nil || !created {
			t.Fatalf("create lead: created=%v err=%v", created, err)
		}
		id := ids.From[ids.LeadKind](ids.UUID(row.Id))
		return referenceFixture{
			id.UUID, e.Contacts.LeadReferences, e.Contacts.LeadLabels,
			func(ctx context.Context) (crmcontracts.RecordReference, error) {
				row, err := e.Contacts.GetLead(ctx, id, storekit.IncludeArchived)
				return crmcontracts.RecordReference{Id: row.Id, Name: row.FullName, Archived: row.ArchivedAt != nil}, err
			}, func() error {
				_, err := e.Contacts.DisqualifyLead(ctx, id, contacts.DisqualifyLeadInput{})
				return err
			},
		}
	case "deal":
		pipeline, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: name + " pipeline"})
		if err != nil {
			t.Fatal(err)
		}
		pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
		stage, err := e.Deals.CreateStage(ctx, deals.CreateStageInput{PipelineID: pipelineID, Name: "Open", Semantic: "open"})
		if err != nil {
			t.Fatal(err)
		}
		stageID := ids.From[ids.StageKind](ids.UUID(stage.Id))
		row, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: name, PipelineID: pipelineID, StageID: stageID, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.DealKind](ids.UUID(row.Id))
		return referenceFixture{
			id.UUID, e.Deals.DealReferences, e.Deals.DealLabels,
			func(ctx context.Context) (crmcontracts.RecordReference, error) {
				row, err := e.Deals.GetDeal(ctx, id, storekit.IncludeArchived)
				return crmcontracts.RecordReference{Id: row.Id, Name: &row.Name, Archived: row.ArchivedAt != nil}, err
			}, func() error {
				_, err := e.Deals.ArchiveDeal(ctx, id, nil)
				return err
			},
		}
	default:
		company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: name + " customer", Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		row, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: name, CompanyID: ids.From[ids.CompanyKind](ids.UUID(company.Id)), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.ProjectKind](ids.UUID(row.Id))
		return referenceFixture{
			id.UUID, e.Projects.ProjectReferences, e.Projects.ProjectLabels,
			func(ctx context.Context) (crmcontracts.RecordReference, error) {
				row, err := e.Projects.GetProject(ctx, id, storekit.IncludeArchived)
				return crmcontracts.RecordReference{Id: row.Id, Name: &row.Name, Archived: row.ArchivedAt != nil}, err
			}, func() error {
				_, err := e.Projects.ArchiveProject(ctx, id, nil)
				return err
			},
		}
	}
}
