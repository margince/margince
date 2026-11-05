// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A breach on a lead whose owner has left is the same miss as one on an unowned
// lead. The owner's task would be refused, so it goes where that breach goes.
func TestABreachOnALeadItsOwnerLeftReachesTheIntakeSeat(t *testing.T) {
	cases := []struct {
		name, leave string
	}{
		{"deactivated owner", `status = 'deactivated'`},
		{"archived owner", `archived_at = now()`},
		{"agent owner", `is_agent = true`},
		{"invited owner", `status = 'invited'`},
		{"read-seat owner", `seat_type = 'read'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupPromoteConsent(t)
			e.enableFirstResponseSLA(t)
			now := clocktest.Now(t).UTC()
			intake := e.seedColleague(t, "intake@example.test")
			e.nameIntakeSeat(t, intake)
			lead := e.seedLeadCreatedAt(t, "left@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
			e.mustExec(t, `UPDATE app_user SET `+tc.leave+` WHERE id = $1`, e.user)

			if _, err := e.store.ScanLeadSLA(e.ctx, now); err != nil {
				t.Fatal(err)
			}

			if got := targetName(e.breachTargetOf(t, lead)); got != intake.String() {
				t.Errorf("escalation target = %s, want the intake seat %s", got, intake)
			}
		})
	}
}

func TestABreachOnALeadItsOwnerLeftIsAddressedToNobodyWithoutAnIntakeSeat(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	now := clocktest.Now(t).UTC()
	lead := e.seedLeadCreatedAt(t, "left@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))
	e.mustExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.user)

	if _, err := e.store.ScanLeadSLA(e.ctx, now); err != nil {
		t.Fatal(err)
	}

	if target := e.breachTargetOf(t, lead); target != nil {
		t.Errorf("a breach was addressed to %q, a seat that has left", *target)
	}
	if e.breachStampOf(t, lead) == nil {
		t.Error("the breach was not recorded")
	}
}

func TestABreachOnALeadItsOwnerStillHoldsStaysWithTheOwner(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	now := clocktest.Now(t).UTC()
	intake := e.seedColleague(t, "intake@example.test")
	e.nameIntakeSeat(t, intake)
	lead := e.seedLeadCreatedAt(t, "held@example.test", now.Add(-DefaultFirstResponseTarget-time.Hour))

	if _, err := e.store.ScanLeadSLA(e.ctx, now); err != nil {
		t.Fatal(err)
	}

	if got := targetName(e.breachTargetOf(t, lead)); got != e.user.String() {
		t.Errorf("escalation target = %s, want the owner %s", got, e.user)
	}
}

func targetName(target *string) string {
	if target == nil {
		return "nobody"
	}
	return *target
}

func (e *promoteConsentEnv) seedColleague(t *testing.T, email string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.mustExec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`, id, email)
	return id
}

func (e *promoteConsentEnv) mustExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
