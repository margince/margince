// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestImportReviewShowsTheActorsTagEffectsToAReaderWithoutUpdateGrant(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	ctx := importAdmin(e)
	handler, _ := leadSourceHandlers(t, e)
	run := testedImportRun(t, e, handler, migration.ObjectLead, "Email,Full Name,Tags\nada@lovelace.example,Ada,Reviewed import\n",
		map[string]string{"Email": "email", "Full Name": "full_name", "Tags": "tags"}, crmcontracts.ImportOnDuplicateCreate)
	service, id, _ := stagedImportApproval(t, e, run)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["import_run"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	grant := perms.Objects["lead"]
	grant.Update = false
	perms.Objects["lead"] = grant
	perms.FieldMasks = []principal.FieldMask{{Object: "lead", Field: "full_name", Condition: principal.MaskOutsideWriteAuthority}}
	reviewer := e.As(e.Rep1, nil, perms)
	proposal, err := service.GetWire(reviewer, id)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.ImportRows == nil {
		t.Fatalf("actor's valid plan became unavailable to its reader: %+v", proposal.Preview)
	}
	found := false
	for _, row := range *proposal.Preview.ImportRows {
		for _, detail := range row.Details {
			found = found || detail.Field == "import_apply_tag" && detail.Value.Text == "Reviewed import"
		}
	}
	if !found {
		t.Fatalf("execution actor's tag effect was hidden by the reader's update grant: %+v", proposal.Preview.ImportRows)
	}
	if _, err := service.Decide(ctx, id, false, nil); err != nil {
		t.Fatal(err)
	}
	perms.RowScope = principal.RowScopeOwn
	reviewer = e.As(e.Rep1, nil, perms)
	decided, err := service.GetWire(reviewer, id)
	if err != nil || decided.ProposedChange == nil {
		t.Fatalf("deciding hid a plan its reader could see: %+v, %v", decided, err)
	}
	perms.FieldMasks[0].Condition = principal.MaskAlways
	masked, err := service.GetWire(e.As(e.Rep1, nil, perms), id)
	if err != nil || masked.ProposedChange != nil || (masked.Summary != nil && *masked.Summary != "") {
		t.Fatalf("a permanently masked field remained readable: %+v, %v", masked, err)
	}
}
