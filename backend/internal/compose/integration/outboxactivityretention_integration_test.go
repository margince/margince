// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// callWithAgedEvent logs a call on a contact through the real writer. It ages
// the event the call emitted past the outbox window and marks it read.
func callWithAgedEvent(t *testing.T, e *Env, subject string, contact ids.UUID) (call, event ids.UUID) {
	t.Helper()
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "call", Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	call = ids.UUID(logged.Id)
	event = ids.MustParse(e.WsScalar(t, `UPDATE event_outbox
		SET published_at = now() - interval '31 days', created_at = now() - interval '31 days', assurance_inputs_read = true
		WHERE id = (SELECT id FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text ORDER BY id DESC LIMIT 1)
		RETURNING id::text`, call))
	return call, event
}

// The statutory floor protects the activity row. A delivered event about it
// is no copy of its content, so it leaves after its window like any other.
func TestAnActivitysEventLeavesAfterItsWindowWhileTheFloorKeepsTheActivity(t *testing.T) {
	e := Setup(t)
	brief, briefEvent := callWithAgedEvent(t, e, "Order confirmed", e.SeedContact(t, "Floor Party", nil))
	heldContact := e.SeedContact(t, "Held Caller", nil)
	e.WsExec(t, `UPDATE contact SET legal_hold = true WHERE id = $1`, heldContact)
	_, heldEvent := callWithAgedEvent(t, e, "Disputed delivery", heldContact)
	e.WsExec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, decided_by_name, reason, qualified_at)
		VALUES ($1, 'controller_pin', 'Test Controller', 'Statutory retention', now())`, brief)
	e.WsExec(t, `UPDATE activity SET retention_class = 'commercial_correspondence', retention_class_at = now(),
		archived_at = now(), restricted_at = now(), restricted_reason = 'statutory retention',
		restricted_until = now() + interval '1 year' WHERE id = $1`, brief)

	if err := compose.NewRetentionServiceFor(e.DB(), nil, slog.Default()).EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
	kept := func(event ids.UUID) bool {
		return e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE id = $1`, event) == 1
	}
	if kept(briefEvent) {
		t.Error("a delivered event about an activity under the floor stayed past its window")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND restricted_at IS NOT NULL`, brief); n != 1 {
		t.Error("the pass touched the activity the floor keeps")
	}
	if !kept(heldEvent) {
		t.Error("the pass removed an event about an activity whose contact is under a legal hold")
	}
}
