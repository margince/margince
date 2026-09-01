// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The Art. 15 export's identifier sections over a real migrated Postgres.
//
// All three sections export ARCHIVED rows alongside live ones, deliberately:
// Art. 15 owes what is HELD, and a retired address, number or channel binding
// is still a record the installation keeps about the subject. That is only
// honest if the export says which is which. Without the archival state the
// package hands a subject a list of identifiers that all read as current, so
// they cannot tell an address they asked to have retired from one this
// installation would still write to — and the section they would use to check
// that the retirement happened is the section that hides it.

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sarIdentifierEnv is one workspace holding one subject who carries a LIVE and
// a RETIRED identifier of each kind the export projects.
type sarIdentifierEnv struct {
	ctx context.Context
	// db is pinned to the workspace this fixture created: the handle is where
	// "which tenant am I" lives now (ADR-0091 §9 step 3), and this env builds
	// its workspace by raw SQL rather than through the installation resolver.
	db *database.DB
	// owner is the migration-role connection the fixture seeded through, kept
	// so a test can add to the subject's history or read the live table
	// definition. Held open for the test's lifetime by the cleanup below.
	owner  *pgx.Conn
	person ids.PersonID
}

// retiredAt is the archival instant every retired row below carries. A fixed
// past timestamp, not now(): the assertions only ask whether the state reached
// the export, and a literal keeps the fixture readable.
var retiredAt = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

func setupSARIdentifiers(t *testing.T) *sarIdentifierEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	// To head before anything else touches this database: testdb.Pool refuses
	// until EnsureSchema has run, and EnsureSchema still REBUILDS whenever it
	// cannot prove the database is a fresh lane clone — so a seed written
	// before it would be dropped rather than reset.
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	ws, user := ids.NewV7(), ids.NewV7()
	person := ids.New[ids.PersonKind]()
	if _, err := owner.Exec(ctx,
		`INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`, user, "admin-"+user.String()+"@sar.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO person (id, full_name, source, captured_by)
		 VALUES ($1, 'Sara Subject', 'manual', 'user:'||$2::text)`,
		person, user); err != nil {
		t.Fatal(err)
	}
	seedIdentifierPairs(ctx, t, owner, person)

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	// Registered where the pool is handed out, before the test adds any cleanup
	// of its own, so it runs last and sees a package that has genuinely stopped.
	// The pool outlives the test now, so a goroutine still holding a connection
	// would go on writing into the database the NEXT test just reset.
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })

	return &sarIdentifierEnv{
		ctx:    exportContext(ws, user),
		db:     database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)),
		owner:  owner,
		person: person,
	}
}

// seedIdentifierPairs gives the subject one live and one retired row of each
// kind. The two values in a pair differ because the live-row uniqueness indexes
// are partial on archived_at IS NULL — reusing one value would leave the export
// unable to say which row it named.
//
// Every value is scoped to the subject through ident(). The dedupe indexes on
// these tables are workspace-wide, not per-person, so a fixed literal binds the
// fixture to being the only one in the database — the second test to seed a
// subject fails on the first test's address rather than on anything it asserts.
func seedIdentifierPairs(ctx context.Context, t *testing.T, owner *pgx.Conn, person ids.PersonID) {
	t.Helper()
	for _, insert := range []struct {
		statement     string
		live, retired string
	}{
		{
			`INSERT INTO person_email (person_id, email, source, captured_by, archived_at)
		  VALUES ($1, $2, 'manual', 'user:test', NULL), ($1, $3, 'manual', 'user:test', $4)`,
			ident(person, liveEmail), ident(person, retiredEmail),
		},
		{
			`INSERT INTO person_phone (person_id, phone, source, captured_by, archived_at)
		  VALUES ($1, $2, 'manual', 'user:test', NULL), ($1, $3, 'manual', 'user:test', $4)`,
			ident(person, livePhone), ident(person, retiredPhone),
		},
		{
			`INSERT INTO person_channel_identity (person_id, provider, channel_user_id, username, source, captured_by, archived_at)
		  VALUES ($1, 'telegram', $2, 'sara', 'connector:telegram', 'connector:telegram', NULL),
		         ($1, 'telegram', $3, 'sara_old', 'connector:telegram', 'connector:telegram', $4)`,
			ident(person, liveAccount), ident(person, retiredAccount),
		},
	} {
		if _, err := owner.Exec(ctx, insert.statement, person, insert.live, insert.retired, retiredAt); err != nil {
			t.Fatal(err)
		}
	}
}

// exportContext is the caller AssembleSAR demands: admin-mediated means the
// person.delete grant AND an unbounded row scope, since the assembly crosses
// every rep's slice on purpose.
func exportContext(ws, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"person": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// The subject's identifiers, one live and one retired per kind. These are the
// SHAPES; ident() below binds each to a particular subject.
const (
	liveEmail      = "sara.live@sar.test"
	retiredEmail   = "sara.retired@sar.test"
	livePhone      = "+493011111111"
	retiredPhone   = "+493022222222"
	liveAccount    = "770000001"
	retiredAccount = "770000002"
)

// ident scopes one identifier to one subject, keeping the shape each column
// expects: an address stays an address and a number stays digits, because the
// dedupe indexes and the column checks are real constraints on these tables.
//
// The discriminator is the subject's own id, so two fixtures in one database
// can hold "the same" identifier without colliding on the workspace-wide
// dedupe indexes. The shape's OWN trailing digits are kept and the
// discriminator goes in the middle — dropping them would fold a fixture's live
// and retired values onto one string, and the pair exists precisely to be
// told apart.
func ident(person ids.PersonID, shape string) string {
	// The TAIL of the id, not its head. These are UUIDv7s, whose leading hex
	// is a millisecond clock — two fixtures built in the same millisecond
	// share that prefix and would collide on exactly the index this exists to
	// avoid. The tail is the random half.
	full := person.String()
	tag := full[len(full)-8:]
	if local, domain, ok := strings.Cut(shape, "@"); ok {
		return local + "+" + tag + "@" + domain
	}
	var digits strings.Builder
	for _, r := range tag {
		digits.WriteString(strconv.Itoa(int(r) % 10))
	}
	// Keep the last four of the shape, which is where liveX and retiredX
	// differ, and splice the subject's digits in front of them.
	keep := len(shape) - 4
	return shape[:keep] + digits.String() + shape[keep:]
}

// TestTheSARExportDistinguishesARetiredBindingFromALiveOne walks all three
// identifier sections, because the obligation is the same in each: the section
// exports archived rows, so it owes the archival state that tells them apart.
func TestTheSARExportDistinguishesARetiredBindingFromALiveOne(t *testing.T) {
	e := setupSARIdentifiers(t)

	pkg, err := AssembleSAR(e.ctx, e.db, e.person)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	for _, section := range []struct {
		name    string
		rows    []map[string]any
		key     string
		live    string
		retired string
	}{
		{"emails", pkg.Emails, "email", ident(e.person, liveEmail), ident(e.person, retiredEmail)},
		{"phones", pkg.Phones, "phone", ident(e.person, livePhone), ident(e.person, retiredPhone)},
		{"channel identities", pkg.ChannelIdentities, "channel_user_id", ident(e.person, liveAccount), ident(e.person, retiredAccount)},
	} {
		t.Run(section.name, func(t *testing.T) {
			byIdentifier := map[string]map[string]any{}
			for _, row := range section.rows {
				identifier, ok := row[section.key].(string)
				if !ok {
					t.Fatalf("a %s row carries no %s: %v", section.name, section.key, row)
				}
				byIdentifier[identifier] = row
			}

			live, ok := byIdentifier[section.live]
			if !ok {
				t.Fatalf("the live %s is missing from the export: %v", section.name, section.rows)
			}
			retired, ok := byIdentifier[section.retired]
			if !ok {
				t.Fatalf("the retired %s is missing from the export — Art. 15 owes what is held: %v", section.name, section.rows)
			}

			state, ok := retired["archived_at"]
			if !ok {
				t.Fatalf("the retired %s exports no archived_at, so it reads as reachable as the live one: %v", section.name, retired)
			}
			if state == nil {
				t.Errorf("the retired %s exports archived_at = NULL, want the retirement instant", section.name)
			}
			if state, ok := live["archived_at"]; !ok || state != nil {
				t.Errorf("the live %s exports archived_at = %v (present: %t), want a present NULL", section.name, state, ok)
			}
		})
	}
}
