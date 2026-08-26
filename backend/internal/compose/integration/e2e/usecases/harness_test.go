// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package usecases

// The harness every scenario boots, and the seat vocabulary they share.
//
// One app per scenario, never shared: apptest.SetupApp* RESETS the package
// database, so a second setup inside a running scenario would delete the
// fixture the first one seeded. That is also why nothing here calls
// t.Parallel.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The seats every scenario needs. Two humans, because the criteria that matter
// most are about the difference between them: an account you own reads
// differently from a colleague's, and "is_you" is only meaningful when someone
// else exists to be not-you.
const (
	repEmail       = "rep@usecases.test"
	repName        = "Sam Rep"
	colleagueEmail = "colleague@usecases.test"
	colleagueName  = "Alex Colleague"
)

// The scope sets a scenario mints a passport with. Named rather than spelled
// at each call site: a scenario that quietly asks for write when it is testing
// a read is a test proving less than it claims.
var (
	scopesRead      = []string{"read"}
	scopesReadWrite = []string{"read", "write"}
)

// scenario is one use case's world: the running app, the assistant's client,
// and the two seats.
type scenario struct {
	*apptest.AppEnv
	// MCP is the assistant. Every tool call in a scenario goes through it.
	MCP *apptest.MCPClient
	// Rep is the human the assistant acts for — the passport's granting seat.
	Rep ids.UUID
	// Colleague is somebody else, so a scenario can seed an account the rep
	// does not own and assert the product says so.
	Colleague ids.UUID
}

// boot brings up a composed app with /mcp mounted and a passport ready.
//
// The connector options are not optional: without WithMCPConnector the handler
// is nil and the /mcp route is never registered, so every call 404s and the
// scenario reads it as a broken test rather than a missing option.
func boot(t *testing.T, scopes []string, extra ...compose.Option) *scenario {
	t.Helper()
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return append([]compose.Option{
			compose.WithMCPConnector(),
			compose.WithMCPResource(origin + "/mcp"),
		}, extra...)
	})
	apptest.BootstrapWorkspaceSession(t, e, "Use Cases", repEmail, repName)

	s := &scenario{AppEnv: e}
	s.Rep = seatIDFor(t, e, repEmail)
	s.Colleague = seedColleague(t, e)
	s.MCP = apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "use-case assistant", scopes...))
	return s
}

// seatIDFor reads a seat's id by email. The bootstrap creates the admin seat
// through the real installation path, so its id is discovered rather than
// chosen.
func seatIDFor(t *testing.T, e *apptest.AppEnv, email string) ids.UUID {
	t.Helper()
	var id ids.UUID
	err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM app_user WHERE lower(email) = lower($1)`, email).Scan(&id)
	})
	if err != nil {
		t.Fatalf("reading the seat for %s: %v", email, err)
	}
	return id
}

// seedColleague adds a second human so ownership has two sides.
//
// Inserted rather than invited: an invitation flow is its own subsystem with
// its own suite, and a scenario about who owns an account should not fail
// because email delivery changed.
func seedColleague(t *testing.T, e *apptest.AppEnv) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO app_user (id, email, display_name, status)
			 VALUES ($1, $2, $3, 'active')`,
			id, colleagueEmail, colleagueName)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the colleague seat: %v", err)
	}
	return id
}

// seed runs one statement inside the workspace, for a fixture that has nothing
// to return.
func (s *scenario) seed(t *testing.T, sql string, args ...any) {
	t.Helper()
	err := apptest.InWorkspace(s.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("seeding: %v\n%s", err, sql)
	}
}

// seedID runs one INSERT whose id is generated here and returned, so a fixture
// can link the rows it just made.
func (s *scenario) seedID(t *testing.T, sql string, args ...any) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	s.seed(t, sql, append([]any{id}, args...)...)
	return id
}

// containsAll says whether every needle appears in the haystack.
//
// Case-insensitive, because these are assertions about whether a caller was
// TOLD something, and a tool that says "Address" has told them.
func containsAll(haystack string, needles ...string) bool {
	lowered := strings.ToLower(haystack)
	for _, needle := range needles {
		if !strings.Contains(lowered, strings.ToLower(needle)) {
			return false
		}
	}
	return true
}

// recordName reads a record's display name out of its fields, for a failure
// message a person can act on.
//
// A record on this surface carries its fields as a raw JSON document rather
// than as named members, so there is no Title to read. A failure naming
// "Rheinufer AG" is worth the decode; one naming a uuid is not.
func recordName(t *testing.T, fields json.RawMessage) string {
	t.Helper()
	var named struct {
		DisplayName string `json:"display_name"`
		FullName    string `json:"full_name"`
	}
	if err := json.Unmarshal(fields, &named); err != nil {
		return "(unnamed record)"
	}
	if named.DisplayName != "" {
		return named.DisplayName
	}
	if named.FullName != "" {
		return named.FullName
	}
	return "(unnamed record)"
}
