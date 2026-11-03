// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// What an activity's links disclose to a role that may not read the linked
// record's TYPE. A deal is read by every seat's row scope, so the row scope
// alone answers yes for it; only the object grant can say no.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const grantedDealName = "Northwind renewal 2027"

// asWithoutDealRead is the rep of as() on a role an admin narrowed to drop
// deal read, keeping everything this read needs.
func (e *promiseEnv) asWithoutDealRead(t *testing.T) context.Context {
	t.Helper()
	ctx := e.as()
	p, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("as() binds no actor")
	}
	objects := make(map[string]principal.ObjectGrant, len(p.Permissions.Objects))
	for object, grant := range p.Permissions.Objects {
		if object != "deal" {
			objects[object] = grant
		}
	}
	p.Permissions.Objects = objects
	return principal.WithActor(ctx, p)
}

// seedDealTask writes one open task assigned to the rep and filed under a
// contact the rep owns and under a deal.
func (e *promiseEnv) seedDealTask(t *testing.T) (taskID, contactID, dealID ids.UUID) {
	t.Helper()
	taskID, contactID, dealID = ids.NewV7(), ids.NewV7(), ids.NewV7()
	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'open', $3, $4, $5, 'seed', 'system')`, dealID, grantedDealName, e.other, pipeline, stage)
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, $2, $3, 'seed', 'system')`, contactID, visibleContactNam, e.rep)
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, due_at, assignee_id, is_done, source, captured_by)
		VALUES ($1, 'task', 'Send the renewal quote', now(), now() + interval '1 day',
			$2, false, 'seed', 'system')`, taskID, e.rep)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), taskID, contactID)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), taskID, dealID)
	return taskID, contactID, dealID
}

// aboutOf is what the open-promise read says the task is about, by entity id.
func aboutOf(ctx context.Context, t *testing.T, e *promiseEnv, taskID ids.UUID) map[ids.UUID]string {
	t.Helper()
	tasks, _, err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).ListOpenTasks(ctx, ListOpenTasksInput{})
	if err != nil {
		t.Fatalf("listing open tasks: %v", err)
	}
	for _, task := range tasks {
		if task.ID == taskID {
			named := map[ids.UUID]string{}
			for _, about := range task.About {
				named[about.EntityID] = about.Name
			}
			return named
		}
	}
	t.Fatal("the task assigned to the caller and filed under their own contact is missing")
	return nil
}

func TestAPromiseNamesALinkedDealOnlyToASeatThatMayReadDeals(t *testing.T) {
	e := setupPromises(t)
	taskID, contactID, dealID := e.seedDealTask(t)

	restricted := aboutOf(e.asWithoutDealRead(t), t, e, taskID)
	if name, told := restricted[dealID]; told {
		t.Errorf("a role without deal read is told the task is about deal %s (%q)", dealID, name)
	}
	if restricted[contactID] != visibleContactNam {
		t.Errorf("a role without deal read lost the contact it may read: about = %v", restricted)
	}

	granted := aboutOf(e.as(), t, e, taskID)
	if granted[dealID] != grantedDealName {
		t.Errorf("a role holding deal read is not told the deal: about = %v", granted)
	}
}

func TestAnActivityReadListsALinkedDealOnlyToASeatThatMayReadDeals(t *testing.T) {
	e := setupPromises(t)
	taskID, contactID, dealID := e.seedDealTask(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))

	linked := func(ctx context.Context) map[ids.UUID]bool {
		got, err := store.GetActivity(ctx, ids.From[ids.ActivityKind](taskID), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("reading the task: %v", err)
		}
		out := map[ids.UUID]bool{}
		if got.Links != nil {
			for _, link := range *got.Links {
				out[ids.UUID(link.EntityId)] = true
			}
		}
		return out
	}

	restricted := linked(e.asWithoutDealRead(t))
	if restricted[dealID] {
		t.Errorf("links[] names deal %s to a role without deal read", dealID)
	}
	if !restricted[contactID] {
		t.Errorf("links[] lost the contact a role without deal read may see: %v", restricted)
	}
	if !linked(e.as())[dealID] {
		t.Error("links[] does not name the deal to a role holding deal read")
	}
}
