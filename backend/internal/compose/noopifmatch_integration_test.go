// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// noopRoomAdminPerms holds the deal room grant the harness admin lacks.
var noopRoomAdminPerms = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"deal":      {Create: true, Read: true, Update: true, Delete: true},
		"deal_room": {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// noopUpdate is one module's update, sent with a body that changes nothing, so
// the store answers without writing.
type noopUpdate struct {
	name string
	// seed creates the record and returns its version.
	seed func(t *testing.T) int64
	// sendEmpty sends the update that changes nothing, under ifVersion.
	sendEmpty func(ifVersion int64) error
}

// An update that changes nothing still answers its If-Match: a stale version
// is version skew, and only the version the row holds goes through.
func TestAnUpdateThatChangesNothingStillRefusesAStaleIfMatch(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()

	for _, tc := range noopUpdates(admin, t, e) {
		t.Run(tc.name, func(t *testing.T) {
			version := tc.seed(t)
			if err := tc.sendEmpty(version - 1); !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("no-op update under a stale If-Match: err = %v, want ErrVersionSkew", err)
			}
			if err := tc.sendEmpty(version); err != nil {
				t.Fatalf("no-op update under the current If-Match: %v", err)
			}
		})
	}
}

func noopUpdates(admin context.Context, t *testing.T, e *integration.Env) []noopUpdate {
	t.Helper()
	var contactID ids.ContactID
	var companyID ids.CompanyID
	var projectID ids.ProjectID
	var dealID ids.DealID
	var stageID openStage
	var roomID ids.DealRoomID
	rooms := dealrooms.NewStore(e.DB())
	roomAdmin := e.As(e.AdminUser, nil, noopRoomAdminPerms)
	return []noopUpdate{
		{
			name: "contact",
			seed: func(t *testing.T) int64 {
				c, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{FullName: "Ilse Brandauer", Source: "manual"})
				if err != nil {
					t.Fatalf("create contact: %v", err)
				}
				contactID = ids.From[ids.ContactKind](ids.UUID(c.Id))
				return *c.Version
			},
			sendEmpty: func(ifVersion int64) error {
				_, err := e.Contacts.UpdateContact(admin, contactID,
					contacts.UpdateContactInput{IfVersion: &ifVersion})
				return err
			},
		},
		{
			name: "company",
			seed: func(t *testing.T) int64 {
				c, err := e.Contacts.CreateCompany(admin, contacts.CreateCompanyInput{DisplayName: "Brandauer GmbH", Source: "manual"})
				if err != nil {
					t.Fatalf("create company: %v", err)
				}
				companyID = ids.From[ids.CompanyKind](ids.UUID(c.Id))
				return *c.Version
			},
			sendEmpty: func(ifVersion int64) error {
				_, err := e.Contacts.UpdateCompany(admin, companyID,
					contacts.UpdateCompanyInput{IfVersion: &ifVersion})
				return err
			},
		},
		{
			name: "project",
			seed: func(t *testing.T) int64 {
				p, err := e.Projects.CreateProject(admin, projects.CreateProjectInput{
					Name: "Brandauer rollout", CompanyID: companyID, Source: "manual",
				})
				if err != nil {
					t.Fatalf("create project: %v", err)
				}
				projectID = ids.From[ids.ProjectKind](ids.UUID(p.Id))
				return *p.Version
			},
			sendEmpty: func(ifVersion int64) error {
				_, err := e.Projects.UpdateProject(admin, projectID,
					projects.UpdateProjectInput{IfVersion: &ifVersion})
				return err
			},
		},
		{
			name: "deal moved to the stage it holds",
			seed: func(t *testing.T) int64 {
				stageID = seedOpenStage(admin, t, e)
				d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
					Name: "Brandauer renewal", PipelineID: stageID.pipeline, StageID: stageID.stage, Source: "manual",
				})
				if err != nil {
					t.Fatalf("create deal: %v", err)
				}
				dealID = ids.From[ids.DealKind](ids.UUID(d.Id))
				return *d.Version
			},
			sendEmpty: func(ifVersion int64) error {
				_, err := e.Deals.AdvanceDeal(admin, dealID,
					deals.AdvanceDealInput{ToStageID: stageID.stage, IfVersion: &ifVersion})
				return err
			},
		},
		{
			name: "deal room archived a second time",
			seed: func(t *testing.T) int64 {
				room, err := rooms.CreateRoom(roomAdmin, dealrooms.CreateRoomInput{
					DealID: dealID, Title: "Brandauer renewal", Source: "manual",
				})
				if err != nil {
					t.Fatalf("create deal room: %v", err)
				}
				roomID = ids.From[ids.DealRoomKind](ids.UUID(room.Id))
				archived, err := rooms.ArchiveRoom(roomAdmin, roomID, nil)
				if err != nil {
					t.Fatalf("archive deal room: %v", err)
				}
				return *archived.Version
			},
			sendEmpty: func(ifVersion int64) error {
				_, err := rooms.ArchiveRoom(roomAdmin, roomID, &ifVersion)
				return err
			},
		},
	}
}

// openStage is a pipeline with one open stage for a deal to sit in.
type openStage struct {
	pipeline ids.PipelineID
	stage    ids.StageID
}

func seedOpenStage(admin context.Context, t *testing.T, e *integration.Env) openStage {
	t.Helper()
	pipeline, err := e.Deals.CreatePipeline(admin, deals.CreatePipelineInput{Name: "Renewals"})
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	stage, err := e.Deals.CreateStage(admin, deals.CreateStageInput{
		PipelineID: pipelineID, Name: "Qualified", Position: 1, Semantic: "open",
	})
	if err != nil {
		t.Fatalf("create stage: %v", err)
	}
	return openStage{pipeline: pipelineID, stage: ids.From[ids.StageKind](ids.UUID(stage.Id))}
}
