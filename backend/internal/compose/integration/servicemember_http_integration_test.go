// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The member an extension's unattended feed runs as.
//
// What an operator relies on, one test each. Nobody can sign in as the member.
// It is free of the seat limit and holds only the landing grants. Only an installation admin
// may make or retire one. It is archived and never deleted. A unit that lands
// records gets its member at boot, once, and a boot never brings back one an
// admin retired.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/pkg/extension"
)

func declareServiceMember(t *testing.T, e *apptest.AppEnv, extension string) userWire {
	t.Helper()
	var member userWire
	if status := e.Call(t, "POST", "/v1/users/service-members", map[string]any{
		"extension": extension, "display_name": "Feed for " + extension,
	}, nil, &member); status != http.StatusCreated {
		t.Fatalf("declaring a service member -> %d, want 201", status)
	}
	return member
}

func TestAServiceMemberHasNoLoginIsOffTheMeterAndHoldsOnlyTheLandingRole(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	before := fullSeatsInUse(t, e)

	member := declareServiceMember(t, e, "alpha")

	if after := fullSeatsInUse(t, e); after != before {
		t.Errorf("seats in use %d -> %d, want no change: a service member is free of the licence seat limit", before, after)
	}
	var passwordSet, isAgent bool
	var seat, extension string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT password_hash IS NOT NULL, is_agent, seat_type, service_extension FROM app_user WHERE id = $1`,
		member.ID).Scan(&passwordSet, &isAgent, &seat, &extension); err != nil {
		t.Fatalf("reading the member: %v", err)
	}
	switch {
	case passwordSet:
		t.Error("the service member has a password, so somebody could sign in as it")
	case !isAgent:
		t.Error("the service member is not an agent row, so sign-in would not refuse it")
	case seat != "full":
		t.Errorf("seat_type = %q, want full", seat)
	case extension != "alpha":
		t.Errorf("service_extension = %q, want the unit it was declared for", extension)
	}
	assertRoles(t, "service member", member, "service_ingest")

	// Signing in by the synthetic address with any password is refused: the
	// member has no password to match.
	if status := e.Call(t, "POST", "/v1/auth/login", map[string]any{
		"email": member.Email, "password": "a-password-that-does-not-exist",
	}, nil, nil); status != http.StatusUnauthorized {
		t.Errorf("signing in as a service member -> %d, want 401", status)
	}
}

func TestOnlyAnInstallationAdminSessionCanDeclareOrArchiveAServiceMember(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	member := declareServiceMember(t, e, "alpha")
	agent := apptest.PassportBearer(t, e, "feed-agent", "read", "write")

	for name, call := range map[string]func() int{
		"declare": func() int {
			return e.Call(t, "POST", "/v1/users/service-members",
				map[string]any{"extension": "beta", "display_name": "Beta feed"}, agent, nil)
		},
		"archive": func() int {
			return e.Call(t, "POST", "/v1/users/service-members/"+member.ID+"/archive", nil, agent, nil)
		},
	} {
		if status := call(); status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Errorf("an agent passport tried to %s a service member -> %d, want 401 or 403", name, status)
		}
	}
}

func TestAServiceMemberIsArchivedAndNeverDeleted(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	member := declareServiceMember(t, e, "alpha")
	declared := fullSeatsInUse(t, e)

	if _, err := e.Owner.Exec(context.Background(), `DELETE FROM app_user WHERE id = $1`, member.ID); err == nil ||
		!strings.Contains(err.Error(), "archived, never deleted") {
		t.Fatalf("deleting a service member -> %v, want the database to refuse it", err)
	}

	if status := e.Call(t, "POST", "/v1/users/service-members/"+member.ID+"/archive", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("archiving the service member -> %d, want 204", status)
	}
	var status string
	var archived bool
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT status, archived_at IS NOT NULL FROM app_user WHERE id = $1`, member.ID).Scan(&status, &archived); err != nil {
		t.Fatalf("the archived member's row is gone: %v", err)
	}
	if status != "deactivated" || !archived {
		t.Errorf("after archiving: status=%q archived=%v, want a deactivated, archived row", status, archived)
	}
	if after := fullSeatsInUse(t, e); after != declared {
		t.Errorf("seats in use %d -> %d after archiving, want no change", declared, after)
	}
	if status := e.Call(t, "POST", "/v1/users/service-members/"+member.ID+"/archive", nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("archiving it twice -> %d, want 404", status)
	}
}

// A colleague is not a service member. The archive route must not retire a
// colleague, or it would become a way to remove one.
func TestArchiveServiceMemberRefusesAnOrdinaryMember(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	var me struct {
		User userWire `json:"user"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK || me.User.ID == "" {
		t.Fatalf("reading the session's own member -> %d, id %q", status, me.User.ID)
	}
	if status := e.Call(t, "POST", "/v1/users/service-members/"+me.User.ID+"/archive", nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("archiving a colleague through the service-member route -> %d, want 404", status)
	}
}

func TestAUnitThatLandsRecordsGetsItsServiceMemberAtBootOnce(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	lands := extension.Extension{Name: "feeder", Ingress: []extension.IngressSource{
		{System: "feeder", Lands: []extension.RecordKind{extension.KindActivity}},
	}}
	quiet := extension.Extension{Name: "quiet"}
	boot := func() {
		t.Helper()
		if err := compose.EnsureExtensionServiceMembers(ctx, e.Pool, logger, []extension.Extension{lands, quiet}); err != nil {
			t.Fatalf("EnsureExtensionServiceMembers: %v", err)
		}
	}
	members := func(unit string) int {
		t.Helper()
		var n int
		if err := e.Owner.QueryRow(ctx,
			`SELECT count(*) FROM app_user WHERE service_extension = $1`, unit).Scan(&n); err != nil {
			t.Fatalf("counting the members of %s: %v", unit, err)
		}
		return n
	}

	boot()
	boot()
	if got := members("feeder"); got != 1 {
		t.Fatalf("a unit that lands records has %d service members after two boots, want 1", got)
	}
	if got := members("quiet"); got != 0 {
		t.Errorf("a unit that lands nothing has %d service members, want none", got)
	}
	var isAgent bool
	var role string
	if err := e.Owner.QueryRow(ctx, `
		SELECT u.is_agent, r.key FROM app_user u
		JOIN role_assignment ra ON ra.user_id = u.id JOIN role r ON r.id = ra.role_id
		WHERE u.service_extension = 'feeder'`).Scan(&isAgent, &role); err != nil {
		t.Fatalf("reading the made member: %v", err)
	}
	if !isAgent || role != "service_ingest" {
		t.Errorf("made member: is_agent=%v role=%q, want an agent row holding service_ingest", isAgent, role)
	}

	// An admin retired it; the next boot must not bring it back.
	if _, err := e.Owner.Exec(ctx,
		`UPDATE app_user SET status = 'deactivated', archived_at = now() WHERE service_extension = 'feeder'`); err != nil {
		t.Fatalf("archiving the member: %v", err)
	}
	boot()
	if got := members("feeder"); got != 1 {
		t.Errorf("a boot after the admin archived the member left %d rows, want the one archived row", got)
	}
}
