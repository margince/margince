// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestImportApprovalCannotAuthorizeForecastCountsAsExactChanges(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["import_run"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	store := migration.NewRunStore(e.DB())
	run, err := store.CreateDraftRun(ctx, migration.CreateDraftRunInput{SourceRef: "ws/import/" + ids.NewV7().String(), Source: "import_api", FileName: "leads.csv", Profile: json.RawMessage(`{"files":[{"name":"leads.csv","row_count":3}]}`), Mapping: migration.RunMapping{Object: migration.ObjectLead, Fields: map[string]string{"Email": "email"}, SourceKey: "Email"}})
	if err != nil {
		t.Fatal(err)
	}
	queued, _, err := store.QueueValidation(ctx, run.ID, run.Version, migration.RunMapping{}, func(context.Context, pgx.Tx, migration.Run) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishValidation(ctx, migration.Lease{Run: run.ID, Status: migration.StatusValidating, Attempt: queued.JobAttempt}, migration.Report{Imported: 3}); err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	for _, rest := range []bool{false, true} {
		payload := map[string]any{"run_id": run.ID}
		if rest {
			payload = map[string]any{"operation": "approveImportRun", "path": "/v1/imports/" + ids.UUID(run.ID).String() + "/approve", "body": nil}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		raw, hash, err := diffhash.Canonical(encoded)
		if err != nil {
			t.Fatal(err)
		}
		id, err := svc.Stage(ctx, approvals.StageInput{Kind: "commit_import", TargetType: "import_run", TargetID: ids.UUID(run.ID), ProposedChange: raw, DiffHash: hash})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := svc.GetWire(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		p := proposal.Preview
		if p == nil || p.State != "incomplete" || p.Details != nil || p.Conflicts == nil || len(*p.Conflicts) != 1 || (*p.Conflicts)[0].Kind != "import_scope_unavailable" {
			t.Fatalf("forecast presented as approvable: %+v", p)
		}
		_, err = svc.Decide(ctx, id, true, nil)
		if _, ok := errors.AsType[*approvals.PreviewContextError](err); !ok {
			t.Fatalf("decision did not enforce scope refusal: %v", err)
		}
		if _, err := svc.Decide(ctx, id, false, nil); err != nil {
			t.Fatalf("refusal blocked rejection: %v", err)
		}
	}
	current, err := store.GetStaged(ctx, run.ID)
	if err != nil || current.Status != migration.StatusAwaitingApproval {
		t.Fatalf("approval started import: %+v / %v", current, err)
	}
}
