// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An installation adds no seat until it has described its own company, because
// the client treats every non-admin seat as proof that it has. Asked of the real
// composition, so the answer is the one contacts gives, not a stand-in.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

var seatRoutes = []struct{ path, email string }{
	{"/v1/users", "invitee@undescribed.test"},
	{"/v1/users/former", "departed@undescribed.test"},
}

func seatBody(email string) map[string]any {
	return map[string]any{"email": email, "display_name": "Some Colleague", "role": "rep"}
}

func seatRowsFor(t *testing.T, e *apptest.AppEnv, email string) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM app_user WHERE email = $1`, email).Scan(&n); err != nil {
		t.Fatalf("counting seats for %s: %v", email, err)
	}
	return n
}

func TestNoSeatIsAddedUntilTheCompanyIsDescribedOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	for _, route := range seatRoutes {
		var refusal refusalWire
		if status := e.Call(t, http.MethodPost, route.path, seatBody(route.email), nil, &refusal); status != http.StatusConflict {
			t.Fatalf("POST %s before the company is described → %d, want 409", route.path, status)
		}
		assertActionableRefusal(t, "POST "+route.path, refusal, "company_not_described")
		if n := seatRowsFor(t, e, route.email); n != 0 {
			t.Errorf("POST %s was refused yet wrote %d seat(s)", route.path, n)
		}
	}

	e.DescribeCompany(t)
	for _, route := range seatRoutes {
		if status := e.Call(t, http.MethodPost, route.path, seatBody(route.email), nil, nil); status != http.StatusCreated {
			t.Errorf("POST %s once the company is described → %d, want 201", route.path, status)
		}
	}
}

// RBAC before the company question, so a caller without user_admin.create
// learns nothing about whether this installation has described itself.
func TestARepIsRefusedASeatBeforeTheCompanyIsDescribed(t *testing.T) {
	assertRepMayAddNoSeat(t, false)
}

func TestARepIsRefusedASeatOnceTheCompanyIsDescribed(t *testing.T) {
	assertRepMayAddNoSeat(t, true)
}

func assertRepMayAddNoSeat(t *testing.T, described bool) {
	t.Helper()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	if described {
		e.DescribeCompany(t)
	}
	demoteToRep(t, e)
	for _, route := range seatRoutes {
		var refusal refusalWire
		if status := e.Call(t, http.MethodPost, route.path, seatBody(route.email), nil, &refusal); status != http.StatusForbidden {
			t.Errorf("a rep POST %s → %d %q, want 403", route.path, status, refusal.Code)
		}
	}
}
