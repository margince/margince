// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestProjectCreationApprovalMatchesTheWriter(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Project Buyer", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			fields := map[string]any{"name": "Buyer rollout", "source": "manual", "company_id": company.Id, "started_at": "2030-01-02", "target_end_date": "2030-06-30"}
			payload := map[string]any{"record_type": "project", "fields": fields}
			if rest {
				payload = map[string]any{"operation": "createProject", "body": fields}
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "project", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil || !slices.Contains(*preview.Effects, "create_project_record") || !slices.Contains(*preview.Effects, "assign_project_key") {
				t.Fatalf("preview: %+v", preview)
			}
			details := map[string]crmcontracts.ApprovalPreviewValue{}
			for _, detail := range *preview.Details {
				details[detail.Field] = detail.Value
			}
			if details["name"].Text != "Buyer rollout" || details["company_id"].Text != company.DisplayName || details["phase"].Text != projects.PhaseInitiative {
				t.Fatalf("missing project context: %+v", details)
			}
			if details["started_at"].Kind != "date" || details["target_end_date"].Kind != "date" {
				t.Fatalf("untyped dates: %+v", details)
			}
			if _, exists := details["key"]; exists {
				t.Fatal("preview claimed a reserved project key")
			}
			if count := e.WsCount(t, `SELECT count(*) FROM project`); count != 0 {
				t.Fatalf("preview wrote %d projects", count)
			}

			restricted := perms
			restricted.Objects = maps.Clone(perms.Objects)
			delete(restricted.Objects, "company")
			denied, err := newApprovalPreviewReader(e.DB()).read(e.As(e.AdminUser, nil, restricted), proposal)
			if err != nil {
				t.Fatal(err)
			}
			if denied == nil || denied.State != previewIncompleteState || denied.Details != nil || denied.Effects != nil {
				t.Fatalf("company context disclosed: %+v", denied)
			}
			input, err := projectCreationRequest(proposal)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := ProjectsStoreOver(e.DB()).CreateProject(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if actual.Name != details["name"].Text || (actual.Phase == nil || string(*actual.Phase) != details["phase"].Text) || actual.Key == nil || *actual.Key == "" || actual.CompanyId == nil || *actual.CompanyId != company.Id {
				t.Fatalf("writer differs: %+v", actual)
			}
			if actual.Companies == nil || len(*actual.Companies) != 1 || (*actual.Companies)[0].Role != projects.CompanyRoleCustomer {
				t.Fatalf("missing customer edge: %+v", actual.Companies)
			}
			project := ids.From[ids.ProjectKind](ids.UUID(actual.Id))
			if count := e.WsCount(t, `SELECT count(*) FROM project_phase_history WHERE project_id=$1`, project); count != 1 {
				t.Fatalf("phase history: %d", count)
			}
		})
	}
}
