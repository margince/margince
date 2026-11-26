// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestRoutingAHandoffRefusesAnInactiveReceiverIncludingAnInheritedOne(t *testing.T) {
	e := setupPromoteConsent(t)
	former := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO app_user(id,email,display_name) VALUES(@id,'former@example.test','Former colleague')`, pgx.StrictNamedArgs{"id": former}); err != nil {
		t.Fatal(err)
	}
	lead := e.seedLead(t, "handover@example.test")
	handoff, err := e.store.SubmitHandoff(e.ctx, NewSDRHandoff{LeadID: leadPtr(lead), AssignedTo: &former})
	if err != nil {
		t.Fatal(err)
	}
	reason := e.someHandoffReason(t, HandoffRecycled)
	if err := e.store.DecideHandoff(e.ctx, handoff, HandoffDecision{Status: HandoffRecycled, ReasonID: &reason}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE app_user SET status='deactivated' WHERE id=@id`, pgx.StrictNamedArgs{"id": former}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.SubmitHandoff(e.ctx, NewSDRHandoff{LeadID: leadPtr(lead), AssignedTo: &former}); !errors.As(err, new(*values.ParseError)) {
		t.Fatalf("new handoff routed to a former colleague: %v", err)
	}
	if err := e.store.ResubmitHandoff(e.ctx, handoff, nil); !errors.As(err, new(*values.ParseError)) {
		t.Fatalf("inherited a former receiver on resubmission: %v", err)
	}
	if current := e.readHandoff(t, handoff); current.status != HandoffRecycled {
		t.Fatalf("refusal changed the handoff to %s", current.status)
	}
	if err := e.store.ResubmitHandoff(e.ctx, handoff, &e.user); err != nil {
		t.Fatal(err)
	}
	if current := e.readHandoff(t, handoff); current.status != HandoffSubmitted {
		t.Fatalf("active successor could not receive the handoff: %s", current.status)
	}
}
