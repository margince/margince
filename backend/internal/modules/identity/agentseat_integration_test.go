// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Agent identities: that bootstrap writes NONE, and that the one endpoint which
// could give one a password still refuses.
//
// Bootstrap used to seed a first-party runner seat (seed-and-fixtures §1.5) for
// a single consumer — the extension-job dispatcher, which resolved it to name a
// tick's initiator. The tick now answers as the job it is, so the row's only
// remaining effects were a full licence seat on every installation and a row in
// the admin roster nobody could act on correctly. It is not seeded any more.
//
// The REFUSALS stay, and this file is where that decision is held. `is_agent`
// remains a supported column — overlay's mappable-seat predicate and
// federatedidentity's sign-in refusal both filter on it, and a resident runner
// will land under it — so the rules about what an agent row may carry are still
// live rules. They are simply no longer exercised by a row the product creates,
// which is why the fixture below writes its own.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const agentSeatAdminPassword = "a bootstrap password!"

// agentSeatRow is an agent identity as the schema holds it. The password is
// asked about rather than read out because absence is the assertion — a hash
// must not be selected into a test's memory to be tested for.
type agentSeatRow struct {
	id          ids.UUID
	email       string
	displayName string
	status      string
	seatType    string
	hasPassword bool
	archived    bool
}

// bootstrapForAgentSeat creates one installation through the real writer and
// returns it with the label its addresses derive from.
func bootstrapForAgentSeat(t *testing.T, pool *pgxpool.Pool) (ids.WorkspaceID, string) {
	t.Helper()
	ctx := context.Background()
	// The test database persists across binary runs, so the label carries the
	// id's random tail to keep every address in this suite unique.
	slug := "agentseat-" + ids.NewV7().String()[24:]
	var wsID ids.WorkspaceID
	err := database.WithInfraTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		wsID, err = createInstallation(ctx, tx, InstallationBootstrap{
			OrganizationName: slug,
			AdminEmail:       "admin@" + slug + ".test",
			AdminName:        "Admin",
			AdminPassword:    agentSeatAdminPassword,
		}, originConfigured, nil, &[]string{})
		return err
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return wsID, slug
}

// readAgentSeats reads every agent identity in the database.
//
// Unqualified on purpose, and safe because setupIdentityDB gives each test its
// own reset database: no table here carries a workspace column to filter on, and
// no policy would narrow the owner connection if it did.
func readAgentSeats(t *testing.T, owner *pgx.Conn) []agentSeatRow {
	t.Helper()
	rows, err := owner.Query(context.Background(),
		`SELECT id, email, display_name, status, seat_type,
		        password_hash IS NOT NULL, archived_at IS NOT NULL
		   FROM app_user WHERE is_agent`)
	if err != nil {
		t.Fatalf("reading the agent seats: %v", err)
	}
	seats, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (agentSeatRow, error) {
		var s agentSeatRow
		return s, r.Scan(&s.id, &s.email, &s.displayName, &s.status,
			&s.seatType, &s.hasPassword, &s.archived)
	})
	if err != nil {
		t.Fatalf("collecting the agent seats: %v", err)
	}
	return seats
}

// seedAgentIdentity writes one agent identity at email and answers its id. It
// is THE fixture for an agent row in this package — every suite whose subject is
// what such a row may not do goes through it.
//
// A direct insert rather than a seam, and that is not a shortcut: no writer in
// the product creates an agent row any more, which is itself what
// TestBootstrapMintsNoAgentSeat asserts. The rules under test are the schema's
// and the service's rules about a row SHAPE, so the fixture's job is to produce
// that shape.
//
// 'full' and 'active' are spelled out because app_user_agent_is_full admits no
// other seat type for an agent, so the row states the constraint it is subject
// to rather than satisfying it by accident.
func seedAgentIdentity(t *testing.T, owner *pgx.Conn, email string) ids.UserID {
	t.Helper()
	id := ids.New[ids.UserKind]()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name, is_agent, seat_type, status)
		 VALUES ($1, $2, 'Margince Agent', true, 'full', 'active')`,
		id, email); err != nil {
		t.Fatalf("seeding an agent identity at %s: %v", email, err)
	}
	return id
}

// theAgentSeat asserts exactly one agent identity is present and returns it.
func theAgentSeat(t *testing.T, owner *pgx.Conn) agentSeatRow {
	t.Helper()
	seats := readAgentSeats(t, owner)
	if len(seats) != 1 {
		t.Fatalf("%d agent identity/identities present, want exactly 1 — this suite seeds its own, "+
			"so any other count means the fixture did not land or bootstrap started seeding one again",
			len(seats))
	}
	return seats[0]
}

// TestBootstrapMintsNoAgentSeat: a fresh installation holds no agent identity.
//
// The absence is the point. A seeded agent is a full seat metered against the
// licence on every installation, for a row nothing in the product reads.
func TestBootstrapMintsNoAgentSeat(t *testing.T) {
	owner, pool := setupIdentityDB(t)
	bootstrapForAgentSeat(t, pool)

	if seats := readAgentSeats(t, owner); len(seats) != 0 {
		t.Fatalf("bootstrap wrote %d agent identity/identities (%q), want none — a seeded agent is a "+
			"full seat metered against the licence on every installation, for a row nothing reads",
			len(seats), seats[0].email)
	}
}

// TestNoSetPasswordLinkCanBeIssuedForAnAgentIdentity holds the refusal that
// outlives the seeded row.
//
// An agent identity, wherever it comes from, is listed in the roster, so an
// admin can reach it with every member action — and one of them mints a
// credential. Login already refuses a row with no hash and forgot-password
// cannot even find it (that lookup requires an existing hash), which leaves the
// admin-issued link as the only way a machine identity could come to hold a
// password.
//
// The fixture seeds its own agent row now. The refusal is NOT retired with the
// seed: `is_agent` is still a supported column, and a resident runner landing
// under it must arrive with this door already shut rather than have it re-opened
// and re-closed.
func TestNoSetPasswordLinkCanBeIssuedForAnAgentIdentity(t *testing.T) {
	owner, pool := setupIdentityDB(t)
	wsID, slug := bootstrapForAgentSeat(t, pool)
	seedAgentIdentity(t, owner, "agent@"+slug+".gradion.local")
	// Bound to the workspace this test just bootstrapped: the suite seeds one
	// per test, so there is no installation singleton to resolve.
	svc := NewServiceFor(database.BindTo(pool, wsID))
	// Workspace AND correlation id, as the HTTP middleware binds both. Without
	// the correlation id the write shape refuses at the audit row, so a missing
	// guard would fail this test on the wrong error and the assertions below
	// would never be reached — the refusal has to be what stops it.
	wsCtx := principal.WithCorrelationID(
		principal.WithWorkspaceID(context.Background(), wsID.UUID), ids.NewV7())

	// The admin's real Identity, resolved the way the HTTP surface resolves it:
	// the refusal has to hold for a caller who passes every other gate.
	admin, _, err := svc.Login(wsCtx, "admin@"+slug+".test", agentSeatAdminPassword)
	if err != nil {
		t.Fatalf("admin login: %v", err)
	}
	seat := theAgentSeat(t, owner)

	_, _, err = svc.IssuePasswordLink(wsCtx, admin, ids.From[ids.UserKind](seat.id))
	if !errors.Is(err, errAgentSeatHasNoPassword) {
		t.Fatalf("issuing a set-password link for an agent identity returned %v, want the agent-seat "+
			"refusal. Redeeming that link would give an identity with no person behind it a working "+
			"credential, and every session opened with it would read as the agent", err)
	}

	// And nothing was written on the way to the refusal: no token to redeem
	// later, and the row still holds no password.
	var tokens int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM auth_token WHERE user_id = $1 AND used_at IS NULL`, seat.id).Scan(&tokens); err != nil {
		t.Fatalf("counting the seat's live tokens: %v", err)
	}
	if tokens != 0 {
		t.Errorf("the refused issue left %d live token(s) for the agent identity; a refusal that "+
			"still mints the credential refuses nothing", tokens)
	}
	if theAgentSeat(t, owner).hasPassword {
		t.Error("the agent identity acquired a password hash during a refused issue")
	}
}
