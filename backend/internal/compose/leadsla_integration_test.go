// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The §18.2 escalation is a cross-module write — a lead breach becomes a task
// activity — so it is proven here, where the edge is wired, against the real
// activities store: the task lands, linked to the lead, assigned to the
// escalation target, and a redelivered event does not land a second one.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// breachedLeadEvent plants one overdue lead in e and returns the
// lead.sla_breached event the escalation consumes, the breach deadline, and the
// system context an automation turn runs under.
//
// Shared by both cases here because they are the same breach seen twice — once
// committing and once refused — and a second hand-written copy of the seed
// would let the two drift into arguing about different leads.
func breachedLeadEvent(t *testing.T, e *integration.Env, owner *pgx.Conn) (ids.UUID, workflow.Event, time.Time, context.Context) {
	t.Helper()
	lead := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
		 VALUES ($1, 'Overdue Lead', 'new', 'inbound', 'human:x', $2)`, lead, e.Rep1); err != nil {
		t.Fatal(err)
	}
	deadline := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	target := openapi_types.UUID(e.Rep1)
	payload, err := json.Marshal(crmcontracts.PublicEventLeadSlaBreached{
		Deadline: deadline, OwnerId: &target, EscalationTarget: &target,
	})
	if err != nil {
		t.Fatal(err)
	}
	ev := workflow.Event{
		ID: ids.NewV7(), Type: "lead.sla_breached", WorkspaceID: e.WS, OccurredAt: deadline,
		Entity:  datasource.EntityRef{Type: datasource.EntityLead, ID: lead},
		Payload: payload,
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	return lead, ev, deadline, ctx
}

// openTasksOnLead counts the lead's open escalation tasks and names their
// assignee, which is what "the task landed" means to the rep who has to work it.
func openTasksOnLead(t *testing.T, owner *pgx.Conn, lead ids.UUID) (int, *ids.UUID) {
	t.Helper()
	var tasks int
	var assignee *ids.UUID
	if err := owner.QueryRow(context.Background(), `
		SELECT count(*), max(a.assignee_id::text)::uuid FROM activity a
		JOIN activity_link l ON l.activity_id = a.id
		WHERE l.lead_id = $1 AND a.kind = 'task' AND NOT a.is_done`, lead).Scan(&tasks, &assignee); err != nil {
		t.Fatal(err)
	}
	return tasks, assignee
}

// leadSLANoticeRecipients lists who the breach put a durable line in front of.
func leadSLANoticeRecipients(t *testing.T, owner *pgx.Conn) []ids.UUID {
	t.Helper()
	rows, err := owner.Query(context.Background(),
		`SELECT recipient_user_id FROM notice WHERE kind = $1 ORDER BY id`, noticeKindLeadSLA)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var recipients []ids.UUID
	for rows.Next() {
		var who ids.UUID
		if err := rows.Scan(&who); err != nil {
			t.Fatal(err)
		}
		recipients = append(recipients, who)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return recipients
}

func TestLeadSLAEscalationLogsOneTaskOnTheLead(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	lead, ev, deadline, ctx := breachedLeadEvent(t, e, owner)

	// Built the way production builds it, and that is the point rather than
	// tidiness: this line used to name the stores it wanted, so when the notify
	// half was added the handler ran here with a nil notices store and panicked
	// inside Apply. A constructor both sites call cannot be half-updated.
	h := newLeadSLAEscalation(e.DB(), func() time.Time { return deadline.Add(time.Hour) })
	eff, err := h.Plan(ctx, ev)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // the bus is at-least-once; the same breach delivered twice is one task
		if _, err := h.Apply(ctx, ev, eff, nil); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}

	tasks, assignee := openTasksOnLead(t, owner, lead)
	if tasks != 1 {
		t.Fatalf("open tasks on the lead = %d, want exactly 1 after two deliveries of one breach", tasks)
	}
	if assignee == nil || *assignee != e.Rep1 {
		t.Errorf("task assignee = %v, want the escalation target %s", assignee, e.Rep1)
	}

	// The notify half, which nothing here asked about until it panicked. A
	// breach the escalation names a target for writes that contact a durable
	// line, addressed to THEM: a notice on somebody else's Worklist is worse
	// than none, because the contact who has to act never sees it.
	recipients := leadSLANoticeRecipients(t, owner)
	// ONE, from the two deliveries the loop above makes. The task half has
	// always been idempotent through its (source_system, source_id) natural
	// key; the notice half now carries the same key, so a breach delivered
	// twice puts one line on one Worklist instead of two identical ones beside
	// a single task — which read as the notice being right and the task lost.
	if len(recipients) != 1 {
		t.Fatalf("%d notices for one breach delivered twice, want 1 — Apply is documented idempotent on its own key",
			len(recipients))
	}
	if recipients[0] != e.Rep1 {
		t.Errorf("notice addressed to %s, want the escalation target %s", recipients[0], e.Rep1)
	}

	// SPELLED OUT, not inferred from the counts above: the two rows are one
	// breach only while they agree on its natural key, and a notice keyed on
	// anything else would still have passed every count here — right up until a
	// redelivery landed a second line beside the same single task.
	wantKey := leadSLATaskSource + ":" + lead.String() + ":" + deadline.Format(time.RFC3339)
	var taskKey, noticeKey string
	if err := owner.QueryRow(context.Background(), `
		SELECT a.source_system || ':' || a.source_id, n.dedupe_key
		FROM activity a
		JOIN activity_link l ON l.activity_id = a.id
		CROSS JOIN notice n
		WHERE l.lead_id = $1 AND a.kind = 'task' AND n.kind = $2`,
		lead, noticeKindLeadSLA).Scan(&taskKey, &noticeKey); err != nil {
		t.Fatal(err)
	}
	if taskKey != wantKey {
		t.Errorf("task natural key = %q, want %q", taskKey, wantKey)
	}
	if noticeKey != wantKey {
		t.Errorf("notice dedupe key = %q, want the task's own %q", noticeKey, wantKey)
	}
}

// failNoticeWrites makes every notice insert raise, which stands in for the
// half of the escalation that can fail after the task has been written — a
// constraint, a lost connection, the process dying between two statements.
//
// A TRIGGER rather than an unresolvable recipient, because both halves point at
// the same seat: a target that no longer exists in app_user breaks the task's
// own assignee foreign key first, so the pair would come out empty for the one
// reason this case is not about.
//
// It is dropped in cleanup: the integration lane resets rows between tests but
// keeps the schema, so a surviving trigger would break every later suite that
// writes a notice.
func failNoticeWrites(t *testing.T, owner *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	if _, err := owner.Exec(ctx, `
		CREATE OR REPLACE FUNCTION notice_write_fault() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
		  RAISE EXCEPTION 'notice write fault injection';
		END $$`); err != nil {
		t.Fatalf("creating the fault-injection function: %v", err)
	}
	// Registered before the trigger is armed, not after both: a failure to arm
	// would otherwise leave the function behind, which is the leak this cleanup
	// exists to prevent. Cleanups run LIFO, so the trigger still drops first.
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP FUNCTION notice_write_fault()`); err != nil {
			t.Errorf("dropping the fault-injection function: %v", err)
		}
	})
	if _, err := owner.Exec(ctx, `
		CREATE TRIGGER notice_write_fault_trigger
		BEFORE INSERT ON notice
		FOR EACH ROW EXECUTE FUNCTION notice_write_fault()`); err != nil {
		t.Fatalf("arming the fault-injection trigger: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TRIGGER notice_write_fault_trigger ON notice`); err != nil {
			t.Errorf("dropping the fault-injection trigger: %v", err)
		}
	})
}

// TestLeadSLAEscalationWritesBothHalvesOrNeither holds the pair.
//
// The automation engine claims a run BEFORE Apply, so a refused notice is never
// redelivered: a task that survives it is a breach a rep is asked to work with
// nothing on their Worklist saying why, and no second delivery will ever supply
// the missing line. Either the breach is recorded whole or it is not recorded.
func TestLeadSLAEscalationWritesBothHalvesOrNeither(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	lead, ev, deadline, ctx := breachedLeadEvent(t, e, owner)
	failNoticeWrites(t, owner)

	h := newLeadSLAEscalation(e.DB(), func() time.Time { return deadline.Add(time.Hour) })
	eff, err := h.Plan(ctx, ev)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Apply(ctx, ev, eff, nil); err == nil {
		t.Fatal("apply reported success while the notice write was refused")
	}

	if tasks, _ := openTasksOnLead(t, owner, lead); tasks != 0 {
		t.Errorf("open tasks on the lead = %d after a refused notice, want 0 — the task committed without the line that explains it", tasks)
	}
	if recipients := leadSLANoticeRecipients(t, owner); len(recipients) != 0 {
		t.Errorf("notices = %d after a refused notice write, want 0", len(recipients))
	}
}
