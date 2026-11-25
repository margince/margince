// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A breached lead nobody owns escalates to the configured intake seat, and
// only to a seat that can work and read leads.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A lead nobody owns is the queue's worst case: past its response target, and
// nobody picked it up. Without an intake seat the escalation reaches no one.
// The configured intake seat answers for it.
func TestAnOwnerlessBreachReachesTheConfiguredIntakeSeat(t *testing.T) {
	t.Parallel()
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	now := time.Now().UTC()

	// Unset: the breach still records, and is addressed to nobody rather than
	// to somebody who never agreed to answer for it.
	silent := e.seedOwnerlessLeadCreatedAt(t, "silent@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
	if _, err := e.store.ScanLeadSLA(e.ctx, now); err != nil {
		t.Fatalf("scan with no configured seat: %v", err)
	}
	if target := e.breachTargetOf(t, silent); target != nil {
		t.Errorf("an unconfigured installation addressed the breach to %q, want nobody", *target)
	}

	// Configured: the same breach now names the seat that answers for it.
	e.nameIntakeSeat(t, e.user)
	addressed := e.seedOwnerlessLeadCreatedAt(t, "addressed@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
	if _, err := e.store.ScanLeadSLA(e.ctx, now); err != nil {
		t.Fatalf("scan with a configured seat: %v", err)
	}
	target := e.breachTargetOf(t, addressed)
	if target == nil || *target != e.user.String() {
		t.Errorf("escalation target = %v, want the configured intake seat %s", target, e.user)
	}
}

// A configured seat that has since been suspended is the same silence as no
// seat at all, wearing a configuration that looks correct.
func TestAnIntakeSeatThatCannotWorkIsTreatedAsUnset(t *testing.T) {
	t.Parallel()
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	now := time.Now().UTC()

	e.nameIntakeSeat(t, e.user)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.user); err != nil {
		t.Fatal(err)
	}

	lead := e.seedOwnerlessLeadCreatedAt(t, "suspended@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
	if _, err := e.store.ScanLeadSLA(e.ctx, now); err != nil {
		t.Fatalf("scan with a suspended seat: %v", err)
	}
	if target := e.breachTargetOf(t, lead); target != nil {
		t.Errorf("a suspended seat was addressed (%q); nobody reads that desk", *target)
	}
}

// grantLeadRead gives a seat the one grant the escalation check asks for. The
// harness seeds no role assignments, so every seat it makes is blind to leads.
// A test that names an intake seat makes it a seat that could answer.
func (e *promoteConsentEnv) grantLeadRead(t *testing.T, seat ids.UUID) {
	t.Helper()
	role := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO role (id, key, name, permissions)
		 VALUES ($1, $2, 'Lead reader', '{"objects":{"lead":{"read":true}}}'::jsonb)`,
		role, "lead_reader_"+role.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO role_assignment (id, role_id, user_id) VALUES ($1, $2, $3)`,
		ids.NewV7(), role, seat); err != nil {
		t.Fatal(err)
	}
}

// nameIntakeSeat writes the setting through the real writer, as an operator
// does, so the test exercises the gate.
func (e *promoteConsentEnv) nameIntakeSeat(t *testing.T, seat ids.UUID) {
	t.Helper()
	e.grantLeadRead(t, seat)
	id := ids.From[ids.UserKind](seat)
	if _, err := e.store.UpdateLeadSettings(e.ctx, UpdateLeadSettingsInput{
		UnassignedEscalationUserID: &id,
	}); err != nil {
		t.Fatalf("naming the intake seat: %v", err)
	}
}

// A seat that cannot read leads is refused as the escalation target, though it
// is a good seat to hand work to. An escalation carries the lead it is about.
// A colleague with activity access and no lead grant would get a task naming
// a record they cannot open, which also discloses it.
func TestAnIntakeSeatMustBeAbleToReadTheLeadsItAnswersFor(t *testing.T) {
	t.Parallel()
	e := setupPromoteConsent(t)

	// A live seat with no role assignment at all: assignable, and blind to
	// leads.
	blind := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'blind@example.test', 'No Grants')`,
		blind); err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.UserKind](blind)
	_, err := e.store.UpdateLeadSettings(e.ctx, UpdateLeadSettingsInput{
		UnassignedEscalationUserID: &id,
	})
	if !errors.As(err, new(*EscalationSeatError)) {
		t.Fatalf("naming a seat that cannot read leads → %v, want EscalationSeatError", err)
	}
	var stored int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM setting WHERE key = $1`, UnassignedEscalationUserID.Key()).
		Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Errorf("the refused nomination was stored anyway")
	}
}
