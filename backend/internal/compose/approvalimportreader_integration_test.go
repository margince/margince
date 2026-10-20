// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/riverqueue/river"
)

func TestImportReviewShowsTheActorsTagEffectsToAReaderWithoutUpdateGrant(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	ctx := importAdmin(e)
	handler, _ := leadSourceHandlers(t, e)
	profile, err := handler.profileAndStore(ctx, migration.ObjectLead, "tagged-leads.csv", []byte("Email,Full Name,Tags\nada@lovelace.example,Ada,Reviewed import\n"))
	if err != nil {
		t.Fatal(err)
	}
	object := crmcontracts.ImportObjectLead
	mapping := map[string]string{"Email": "email", "Full Name": "full_name", "Tags": "tags"}
	if _, err := handler.stageRun(ctx, crmcontracts.CreateImportRunRequest{Connector: crmcontracts.CreateImportRunRequestConnectorCsv, Object: &object, SourceRef: profile.SourceRef, Mapping: &mapping}); err != nil {
		t.Fatal(err)
	}
	runs := migration.NewRunStore(e.DB())
	run, err := runs.GetStaged(ctx, migration.RunID(profile.RunId))
	if err != nil {
		t.Fatal(err)
	}
	args := ImportValidateArgs{Workspace: e.WS, RunID: run.ID, Attempt: run.JobAttempt}
	if err := validateWorkerFor(e, handler).Work(context.Background(), &river.Job[ImportValidateArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]ids.UUID{"run_id": ids.UUID(run.ID)})
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	service := NewApprovalsService(e.DB())
	id, err := service.Stage(ctx, approvals.StageInput{Kind: "commit_import", TargetType: "import_run", TargetID: ids.UUID(run.ID), ProposedChange: canonical, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["import_run"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	grant := perms.Objects["lead"]
	grant.Update = false
	perms.Objects["lead"] = grant
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
}
