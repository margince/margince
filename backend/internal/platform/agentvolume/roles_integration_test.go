// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentvolume

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Roles choose the limit and never the counter. One passport's count stays
// where it is when its human loses Admin or an admin edits a role, so the
// lower limit refuses at once and nothing starts the window again.
func TestARoleChangeKeepsTheCountAndMovesOnlyTheLimit(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{policy: rolePolicy()}
	meter := NewWithClock(redistest.Client(t), Limits{}, DefaultWindow, frozen(&at)).WithPolicySource(src)
	ws, passport := aWorkspace(), aPassport()
	asAdmin := roledCall(t, ws, passport, "admin")
	if err := meter.Consume(asAdmin, Calls, 1500); err != nil {
		t.Fatal(err)
	}
	if r := meter.Read(asAdmin, Calls); r.Exceeded || r.Limit != 20000 {
		t.Fatalf("as an Admin, 1500 calls read %+v, want inside 20000", r)
	}

	asUser := roledCall(t, ws, passport, "rep")
	if r := meter.Read(asUser, Calls); !r.Exceeded || r.Observed != 1500 || r.Limit != DefaultCalls {
		t.Errorf("after losing Admin the same passport reads %+v, want 1500 of %d and refused", r, DefaultCalls)
	}

	src.policy.Roles = map[string]Limits{"admin": {Calls: 1000}}
	meter.ForgetPolicy()
	if r := meter.Read(asAdmin, Calls); !r.Exceeded || r.Observed != 1500 {
		t.Errorf("after the Admin value fell to 1000 the passport reads %+v, want its 1500 kept and refused", r)
	}
}

// One usage read judges each holder against its own roles.
func TestUsageJudgesEachHolderByItsOwnRoles(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	meter := NewWithClock(redistest.Client(t), Limits{}, DefaultWindow, frozen(&at)).
		WithPolicySource(&scriptedSource{policy: rolePolicy()})
	ws, admin, rep := aWorkspace(), aPassport(), aPassport()

	usage, err := meter.Usage(inWorkspace(t), ws, []Holder{
		{Subject: admin.String(), Roles: []string{"admin"}}, {Subject: rep.String(), Roles: []string{"rep"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := usage[admin.String()].Calls.Limit; got != 20000 {
		t.Errorf("the Admin's passport is reported against %d calls, want 20000", got)
	}
	if got := usage[rep.String()].Calls.Limit; got != DefaultCalls {
		t.Errorf("the User's passport is reported against %d calls, want %d", got, DefaultCalls)
	}
}

// roledCall is meteredCall for a passport whose granting human holds roles.
func roledCall(t *testing.T, ws, passport ids.UUID, roles ...string) context.Context {
	t.Helper()
	return principal.WithActor(principal.WithWorkspaceID(t.Context(), ws), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:reader", PassportID: passport,
		Permissions: principal.Permissions{RoleKeys: roles},
	})
}
