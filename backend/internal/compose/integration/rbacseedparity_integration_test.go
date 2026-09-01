// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An RBAC object must reach an EXISTING installation, not just a fresh one.
//
// identity.seedSystemRoles writes each role's permissions document ONCE, at
// workspace creation, and never re-syncs. So an object added to the policy after
// an installation bootstrapped reaches it only through a backfill migration, and
// a backfill that grants the wrong verb, targets the wrong roles, or matches no
// rows is indistinguishable from a correct one until somebody gets a 403.
//
// It compares against migrations/testdata/rbac_seeded_defaults.json rather than
// against policy, and that is not a shortcut: policy sits behind
// identity/internal/, so Go's own import fence rejects reading it from here.
// identity/rbacfixture_test.go is the bridge built for exactly this — it lives
// inside the fence, renders the documents from policy.MustDefaultJSON, and pins
// them to that fixture on every unit pass, so the fixture IS policy's value.
//
// TWO PRE-STATES, because neither one alone answers the obligation:
//
//   - PER WRITE — today's matrix minus that write's own objects. It ISOLATES, so
//     a failure names one migration. The cost is that the writes are never
//     composed, and the state is modern apart from the objects removed, which is
//     a state no installation was ever actually in.
//   - COMPOSED — the documents an installation bootstrapped AT THE BASELINE
//     really held, with every write replayed over them in version order. That state is real, and it is the OLDEST one that can reach
//     head at all, so this is the arm that models the obligation. The cost is
//     that a failure names the sequence rather than the migration.
//
// The composed arm is also the only one that can see an object added to the
// policy with NO backfill at all — for any object added SINCE the baseline. The
// per-write arm cannot see it at any age: it derives its pre-state FROM the
// writes, so an object that no migration mentions is never removed from the
// pre-state and so never missed. An object that predates the baseline sits in
// both sides here too, and is out of reach of both arms; nothing can recover a
// pre-state older than the floor, because no such installation can migrate.

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/migrations"
)

// seededDefaults is the matrix identity seeds, as identity itself renders it.
const seededDefaults = "../../../migrations/testdata/rbac_seeded_defaults.json"

// baselineEraDefaults is the matrix a fresh installation was seeded with at the
// migration baseline — the OLDEST pre-state it is meaningful to prove anything
// about, because dbmigrate.assertLedgerMatches refuses any database whose ledger
// records core 0001 under a different name, and every pre-baseline database
// records it as "foundation". Such a database cannot be repaired forward; the
// migrator says so and tells the operator to rebuild.
//
// DERIVED, not hand-written, and checked elsewhere: backend's
// rbacbaselineerafixture_test.go pins this file to `git show <baseline>:<the
// seeded-matrix fixture>` on every unit pass. That matters because the pre-state
// is exactly what an unwilling author would edit — move the object into the
// starting state and the convergence a broken backfill never delivered is
// already there, with nothing else changed to give it away.
//
// The derivation is checked THERE and not here because it needs full history and
// this lane does not have it: _lane-integration.yml gives
// integration-unit-coverage `fetch-depth: 0` while the integration SHARDS check
// out shallow. Same split as rbac_seeded_defaults.json — identity renders it,
// this package reads it.
const baselineEraDefaults = "../../../migrations/testdata/rbac_baseline_era_defaults.json"

// controlRole is a role the seeded matrix does not name, carrying a document of
// its own, present for every replay below.
//
// It is the missing-predicate detector. A backfill that names no roles at all —
// the easiest hand-written mistake there is — reaches every role row in the
// installation, and every role the fixture names SHOULD receive the grant, so
// without a row that should not, an unscoped UPDATE looks exactly like a correct
// one.
//
// It does NOT detect a write that lists keys and omits `is_system`, and that is
// not a gap: `role_key_unique UNIQUE (key)` means no non-system row can hold a
// key a seeded role already occupies, so a key-listed write cannot reach an
// operator's own role whatever it says about is_system. What the row does catch
// is a write predicated on the DOCUMENT rather than the key — see
// controlDocument, which is shaped for exactly that.
const controlRole = "custom_scoped_role"

// controlDocument holds real object keys and a real row scope ON PURPOSE.
//
// An empty document at row_scope "own" — what this was — is only reachable by a
// statement with no predicate at all, which made the detector's claim much
// narrower than it read. Two shapes slipped past it, and both reach every
// operator-defined role in the installation:
//
//	WHERE (permissions -> 'objects') ? 'deal'        -- empty objects never match
//	WHERE permissions ->> 'row_scope' = 'team'       -- "own" never matches
//
// The second is the sharp one: a backfill widening team to all is a mass row
// disclosure, and a control row at "own" cannot see it. So the row carries a
// non-empty subset of the real vocabulary and sits at "team", which is the
// scope a predicate-driven backfill is most likely to select on. It is still a
// role the seeded matrix does not name, so no backfill should touch it.
var controlDocument = []byte(
	`{"objects": {"deal": {"create": false, "read": true, "update": false, "delete": false}, ` +
		`"person": {"create": false, "read": true, "update": false, "delete": false}}, ` +
		`"row_scope": "team"}`)

// roleDocument is one role's permissions document. Grants stay RawMessage: this
// file owns whether the two paths AGREE, never what a grant should be — the
// second opinion on that is policy's own test, and transcribing it here would
// only add something to drift.
type roleDocument struct {
	Objects  map[string]json.RawMessage `json:"objects"`
	RowScope string                     `json:"row_scope"`
}

func readSeededDefaults(t *testing.T) map[string]roleDocument {
	t.Helper()
	raw, err := os.ReadFile(seededDefaults)
	if err != nil {
		t.Fatalf("reading %s: %v — identity/rbacfixture_test.go writes it with -update", seededDefaults, err)
	}
	var seeded map[string]roleDocument
	if err := json.Unmarshal(raw, &seeded); err != nil {
		t.Fatalf("decoding %s: %v", seededDefaults, err)
	}
	if len(seeded) == 0 {
		t.Fatalf("%s holds no roles — an empty matrix would make every comparison below pass", seededDefaults)
	}
	return seeded
}

// A fresh installation's role documents are the ones the fixture records.
//
// This half needs no backfill to exist, and it is the half nothing was checking:
// the fixture had exactly one reader — the identity test that WRITES it — so it
// was a one-ended pin. A migration that touches `role`, or a seed that stops
// running, makes the code and the database disagree with nothing to report it.
func TestTheRealBootstrapSeedsTheDocumentedMatrix(t *testing.T) {
	seeded := readSeededDefaults(t)
	e := apptest.SetupApp(t)
	ctx := context.Background()

	bootstrapInstallation(t, e)

	live := liveRoleDocuments(ctx, t, e.Owner)
	if len(live) == 0 {
		t.Fatal("the bootstrap wrote no system roles at all")
	}
	// A fresh bootstrap runs no migration, so any marker on it was written by
	// nothing — which is exactly what it should not be.
	assertSameMatrix(t, seeded, live, "the real bootstrap", map[string]bool{})
}

// Every write a migration makes to role.permissions leaves an installation on
// the seeded matrix.
//
// EXECUTED, not scanned. A static check on the JSON payload would pass a
// migration whose WHERE clause targeted the wrong roles, omitted is_system, or
// wrote the wrong jsonb path — the payload is the part least likely to be wrong.
//
// Two shapes, because a permission write is not always a grant:
//
//   - it names `{objects,X}` paths — an installation predating it held today's
//     matrix minus X, so that state is derivable, and the replay must CONVERGE
//     it back onto the matrix.
//   - it writes some other path (`{row_scope}`, say) — the prior value is
//     knowable only by reading the migration's own predicate, which this gate
//     does not do. The weaker claim it can still make is that the writes TAKEN
//     TOGETHER land on the matrix, which catches a write that fires
//     unconditionally and one whose predicate never matches.
func TestEveryRBACBackfillConvergesOnTheSeededMatrix(t *testing.T) {
	seeded := readSeededDefaults(t)
	writes := rolePermissionMigrations(t)

	// NOT a tolerated zero. core carries a live backfill today, so an empty set
	// means the detection below stopped seeing it — and the previous gate for
	// this obligation was lost exactly that way, to a consolidation that folded
	// the migrations it read into a baseline.
	if len(writes) == 0 {
		t.Fatal("no migration writes role.permissions, but core carries at least one — " +
			"the detection in rolePermissionMigrations has gone blind, or a consolidation " +
			"folded the backfills into the baseline and this gate now proves nothing")
	}

	e := apptest.SetupApp(t)
	ctx := context.Background()
	bootstrapInstallation(t, e)

	// IN ORDER, CUMULATIVELY, and compared once at the end.
	//
	// Each replay used to be judged alone, which held while every backfill
	// converged on the same matrix. It stopped holding the moment one of them
	// CHANGED the matrix: 1787449829 pins the seeded roles at own scope and
	// 1788244324 moves the manager to team, so replaying the first by itself
	// now lands on an answer that was right when it was written and is not the
	// answer today. Neither migration is wrong; asking one of them in isolation
	// is.
	//
	// So this asks what an installation actually experiences — every write, in
	// version order, over the documents the baseline left — and requires the
	// END of that to be the seeded matrix. A backfill that diverges is still
	// caught, unless a LATER one happens to correct it, which is exactly the
	// case the isolated reading called a failure and an installation calls
	// Tuesday.
	//
	// The objects every write names are rewound first, together: an
	// installation predating all of them held today's matrix minus all of them.
	var rewound []string
	for _, write := range writes {
		rewound = append(rewound, write.objects...)
	}
	rewindTo(ctx, t, e, rewound)
	for _, write := range writes {
		for _, statement := range write.statements {
			if _, err := e.Owner.Exec(ctx, statement); err != nil {
				t.Fatalf("replaying a role.permissions write from %s: %v\n%s",
					write.name, err, statement)
			}
		}
	}
	assertSameMatrix(t, seeded, liveRoleDocuments(ctx, t, e.Owner), "every backfill in order",
		markerWrites(t, writes))
	assertControlRoleUntouched(ctx, t, e.Owner, "every backfill in order")
}

// Every write COMPOSED, replayed in version order over the documents an
// installation bootstrapped at the baseline actually held.
//
// The convergence arm above cannot make this claim. Its pre-state is derived
// from the writes themselves — the objects they name, rewound out of today's
// matrix — so an installation older than all of them is never the thing under
// test, and neither is a document those writes never mention.
//
// It is also the arm that catches the ORIGINAL defect: an object added to the
// policy with no backfill written for it. An object no migration mentions is
// never absent from a pre-state built out of what the migrations say, so its
// missing backfill is invisible there. Here the pre-state is derived from
// history, independently of what the migrations happen to say, so the object is
// missing at the start and still missing at the end.
//
// Version order and not file order: dbmigrate sorts versions as STRINGS and
// applies them in that order, and rolePermissionMigrations INHERITS that order
// from dbmigrate.Load rather than re-deriving it, so the sequence replayed here
// is the sequence a real upgrade runs.
func TestTheBackfillsComposeFromTheOldestUpgradableInstallation(t *testing.T) {
	seeded := readSeededDefaults(t)
	baselineEra := readBaselineEraDefaults(t)
	writes := rolePermissionMigrations(t)

	// Same reasoning as the per-write arm: core carries backfills today, so an
	// empty set means the detection went blind rather than that there is nothing
	// to compose.
	if len(writes) == 0 {
		t.Fatal("no migration writes role.permissions, but core carries at least one — " +
			"the detection in rolePermissionMigrations has gone blind")
	}
	assertPreStateIsNotAlreadyTheAnswer(t,
		readMatrixAsValues(t, baselineEraDefaults), readMatrixAsValues(t, seededDefaults))

	e := apptest.SetupApp(t)
	ctx := context.Background()
	bootstrapInstallation(t, e)
	rewindToBaselineEra(ctx, t, e, baselineEra)

	for _, write := range writes {
		for _, statement := range write.statements {
			if _, err := e.Owner.Exec(ctx, statement); err != nil {
				t.Fatalf("replaying %s over the baseline-era documents: %v\n%s",
					write.name, err, statement)
			}
		}
	}
	const via = "the composed replay from the baseline era"
	assertSameMatrix(t, seeded, liveRoleDocuments(ctx, t, e.Owner), via, markerWrites(t, writes))
	assertControlRoleUntouched(ctx, t, e.Owner, via)
}

// assertPreStateIsNotAlreadyTheAnswer refuses a pre-state that already equals
// the matrix the replay is supposed to reach.
//
// Without it this arm degrades silently into a second copy of the fresh-install
// test: replaying writes that all no-op over a state that is already correct
// asserts nothing and reports the same word for it, PASS. That happens for a
// real reason rather than a hypothetical one — the day the vocabulary stops
// growing, the baseline-era documents catch up with head — so the gate has to
// say it has run out of distance instead of quietly passing.
func assertPreStateIsNotAlreadyTheAnswer(t *testing.T, before, after map[string]any) {
	t.Helper()
	for _, role := range sortedKeys(after) {
		if _, ok := before[role]; !ok {
			// A role the baseline era did not have. No permissions write can
			// CREATE a role row, so this is a different obligation than the one
			// under test, and it would otherwise surface downstream as a puzzling
			// missing-role error rather than as what it is.
			t.Fatalf("the seeded matrix names role %q and the baseline-era documents do not. "+
				"A role.permissions backfill only ever UPDATEs, so no migration can bring this "+
				"role to an installation that predates it — the seed and the upgrade path have "+
				"diverged, and that is a gap in the migration rather than in this gate", role)
		}
	}
	// BOTH directions, in one comparison. Walking only `after`'s keys meant a
	// role or object the pre-state holds and today's matrix does not — an object
	// retired from the policy — was never examined, no distance was found, and
	// the fatal below fired telling the author to retire an arm that in fact had
	// real distance to cross. DeepEqual over decoded values says "identical" only
	// when they are, which is the claim this guard needs.
	if !reflect.DeepEqual(before, after) {
		return
	}
	t.Fatalf("%s already equals the matrix the replay must reach, so this arm proves nothing: "+
		"every write below can no-op and the comparison still passes.\n"+
		"The baseline-era pre-state has caught up with head. Repoint baselineEraCommit — it lives "+
		"in backend/gates/rbacbaselineerafixture_test.go, which also regenerates this file — at a later "+
		"consolidation floor if one has landed, or delete this arm; do NOT leave it green over "+
		"a comparison with no distance in it.", baselineEraDefaults)
}

// assertSameMatrix compares two role matrices and names every disagreement.
//
// Both directions: a role the database holds and the fixture does not is as much
// a divergence as the reverse, and reporting only one of them is how half a
// defect goes unnoticed.
// markerWrites is the set of version literals the replayed statements actually
// WRITE into the provenance key, so a marker can be held to naming its writer.
//
// Not merely a version that was replayed: the down migration matches the marker
// against the exact version it wrote, so a marker naming a DIFFERENT migration
// — even a real one, even one that ran — leaves the rollback unable to tell a
// scope it widened from one an operator chose, and the manager stays at team
// scope through a rollback that was supposed to narrow it.
//
// The set holds what was WRITTEN rather than which migration wrote it, and the
// two are only the same while every migration stamps its own version. So this
// also fails a statement that stamps somebody else's: a marker is a rollback's
// handle on its own work, and one that names a migration whose down will not
// look for it is a widening nothing narrows back.
//
// Mentioning the key is not writing it. Detection is the jsonb PATH the write
// addresses, which is why 1788244324's down — matching `permissions ->>
// 'row_scope_set_by'` to find what to strip — and the prose above it are not
// mistaken for writers. Naming a non-writer is precisely the false pass this
// set exists to refuse, so it cannot be found by looking for the key anywhere
// in the text.
//
// It reads the spelling the migrations use today. A write addressing the same key
// another way — an ARRAY path, a jsonb concat — is not seen, and the marker it
// leaves is then reported as naming a non-writer. That is the direction to fail
// in: it costs a migration author one line here, where a lenient match costs a
// rollback the ability to tell its own work from an operator's.
func markerWrites(t *testing.T, writes []permissionWrite) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, write := range writes {
		version := strings.SplitN(write.name, "_", 2)[0]
		for _, statement := range write.statements {
			for _, match := range markerWritePattern.FindAllStringSubmatch(statement, -1) {
				if match[1] != version {
					t.Errorf("%s stamps %s into %s, rather than its own version. The down migration "+
						"looks for the version it wrote, so a marker carrying another one is a "+
						"widening its own rollback cannot find", write.name, match[1], rowScopeSetBy)
				}
				out[match[1]] = true
			}
		}
	}
	return out
}

// The jsonb path a write addresses, and the version literal it puts there:
// `jsonb_set(permissions, '{row_scope_set_by}', '"1788244324"'::jsonb, true)`.
var markerWritePattern = regexp.MustCompile(`'\{` + rowScopeSetBy + `\}'\s*,\s*'"([^"]*)"'`)

// rowScopeSetBy is the provenance key. The comparison below reads it, and
// markerWrites looks for the statement that writes it.
const rowScopeSetBy = "row_scope_set_by"

func assertSameMatrix(t *testing.T, want map[string]roleDocument, got map[string]json.RawMessage, via string,
	stamped map[string]bool,
) {
	t.Helper()
	for _, role := range sortedKeys(want) {
		raw, ok := got[role]
		if !ok {
			t.Errorf("%s: role %q is missing from the database; every route it grants answers 403", via, role)
			continue
		}
		// The WHOLE document first, so a key neither side models cannot slip
		// through, and then the parts, because "these two documents differ" is
		// not a message anybody can act on.
		expected, err := json.Marshal(want[role])
		if err != nil {
			t.Fatalf("%s: encoding the expected document for %q: %v", via, role, err)
		}
		if !sameJSON(t, expected, raw) {
			assertSameDocument(t, role, want[role], decodeRoleDocument(t, role, raw), via)
			assertNoUnmodelledKeys(t, role, raw, via, stamped)
		}
	}
	for _, role := range sortedKeys(got) {
		if _, ok := want[role]; !ok {
			t.Errorf("%s: the database holds system role %q, which the seeded matrix does not — "+
				"either the fixture is stale or something granted a role nobody declared", via, role)
		}
	}
}

// decodeRoleDocument decodes one live document for the detailed comparison.
func decodeRoleDocument(t *testing.T, role string, raw json.RawMessage) roleDocument {
	t.Helper()
	var document roleDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decoding the live document for %q: %v", role, err)
	}
	return document
}

// assertNoUnmodelledKeys names a document key roleDocument does not model, so a
// whole-document difference that the per-part comparison cannot explain still
// reports something actionable rather than reading as a spurious failure.
//
// field_masks is the live case: policy.Document declares it, this file does not
// model it, and it is a privacy control — so a replay that introduced or dropped
// one has to be visible even though no per-field check covers it.
func assertNoUnmodelledKeys(t *testing.T, role string, raw json.RawMessage, via string, stamped map[string]bool) {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("decoding the live document for %q: %v", role, err)
	}
	for key := range keys {
		if key == "objects" || key == "row_scope" {
			continue
		}
		// PROVENANCE, not permission — and JUDGED rather than waved through.
		//
		// row_scope_set_by records WHICH migration widened a seeded role, so
		// the down migration can tell a scope it moved from one an operator
		// chose themselves: matching on the value alone cannot, since both look
		// identical afterwards and a rollback would narrow a setting it never
		// made. It grants nothing and masks nothing, and a migrated
		// installation carries it where a fresh one does not — so comparing it
		// against the seeded document would report that difference as a
		// PERMISSION difference, which is the one thing this file never says.
		//
		// What it must still not be is a free-text field nobody reads: a marker
		// naming a version that does not exist is a rollback the down migration
		// cannot reason about. So the value has to name one of the migrations
		// actually replayed.
		if key == rowScopeSetBy {
			var version string
			if err := json.Unmarshal(keys[key], &version); err != nil || !stamped[version] {
				t.Errorf("%s: role %q was widened by %s, which is not the migration that WRITES that "+
					"marker — the down migration matches it against the exact version it wrote, so a "+
					"marker naming another one leaves a rollback unable to tell a scope it widened "+
					"from one an operator chose", via, role, keys[key])
			}
			continue
		}
		t.Errorf("%s: role %q carries document key %q, which this comparison does not model: %s.\n"+
			"policy.Document declares field_masks as well as objects and row_scope; a replay that "+
			"writes one has to be judged rather than dropped, because field masking is a privacy "+
			"control.", via, role, key, keys[key])
	}
}

func assertSameDocument(t *testing.T, role string, want, got roleDocument, via string) {
	t.Helper()
	if want.RowScope != got.RowScope {
		t.Errorf("%s: role %q has row_scope %q, want %q — the scope decides which ROWS the role sees, "+
			"so a wrong one is a disclosure or a blackout rather than a missing button",
			via, role, got.RowScope, want.RowScope)
	}
	for _, object := range sortedKeys(want.Objects) {
		live, ok := got.Objects[object]
		if !ok {
			t.Errorf("%s: role %q holds no grant on %q, so every %s route answers 403 for it — "+
				"permanently, on any installation that took this path", via, role, object, object)
			continue
		}
		if !sameJSON(t, want.Objects[object], live) {
			t.Errorf("%s: role %q on %q has %s, want %s", via, role, object, live, want.Objects[object])
		}
	}
	for _, object := range sortedKeys(got.Objects) {
		if _, ok := want.Objects[object]; !ok {
			t.Errorf("%s: role %q holds a grant on %q that the seeded matrix does not give it: %s.\n"+
				"A backfill that grants MORE than the policy declares is the mirror of a missing one, "+
				"and it is the direction nobody notices.", via, role, object, got.Objects[object])
		}
	}
}

// assertControlRoleUntouched is the missing-predicate detector: a write that
// names no roles, or omits is_system, reaches this row too.
func assertControlRoleUntouched(ctx context.Context, t *testing.T, conn *pgx.Conn, via string) {
	t.Helper()
	var live json.RawMessage
	if err := conn.QueryRow(ctx,
		`SELECT permissions FROM role WHERE key = $1`, controlRole).Scan(&live); err != nil {
		t.Fatalf("%s: reading the control role back: %v", via, err)
	}
	if !sameJSON(t, controlDocument, live) {
		t.Errorf("%s: the write reached %q, a role the seeded matrix does not name and no backfill "+
			"should touch: %s, want %s.\nA statement that names no roles, or omits is_system, "+
			"grants the object to every role row in the installation.",
			via, controlRole, live, controlDocument)
	}
}

// sameJSON compares two grants as decoded values, so key order and whitespace do
// not read as a difference.
//
// reflect.DeepEqual and not a formatted comparison: `%v` erases JSON types, so
// `{"read": true}` and `{"read": "true"}` both render as `map[read:true]` and a
// backfill that wrote a verb as a string would pass. That is the defect class
// this file exists to catch, so the comparison has to keep the distinction while
// staying blind to key order — which DeepEqual does and formatting does not.
func sameJSON(t *testing.T, want, got json.RawMessage) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(want, &a); err != nil {
		t.Fatalf("decoding the expected grant %s: %v", want, err)
	}
	if err := json.Unmarshal(got, &b); err != nil {
		t.Fatalf("decoding the live grant %s: %v", got, err)
	}
	return reflect.DeepEqual(a, b)
}

func liveRoleDocuments(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]json.RawMessage {
	t.Helper()
	rows, err := conn.Query(ctx, `SELECT key, permissions FROM role WHERE is_system ORDER BY key`)
	if err != nil {
		t.Fatalf("reading the system roles: %v", err)
	}
	defer rows.Close()

	// RAW, so the comparison sees the WHOLE document. roleDocument models two of
	// its keys, and policy.Document already declares a third — field_masks, a
	// privacy control — so decoding here made a replay that dropped or rewrote
	// field masks invisible to every arm. Seeding was fixed to carry raw bytes;
	// this is the reading half of the same defect.
	live := map[string]json.RawMessage{}
	for rows.Next() {
		var key string
		var document json.RawMessage
		if err := rows.Scan(&key, &document); err != nil {
			t.Fatalf("decoding a role document: %v", err)
		}
		live[key] = document
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the system roles: %v", err)
	}
	return live
}

// rewindTo puts the bootstrapped installation back to the state one predating
// this write was in: its own documents, minus the objects the write grants.
//
// A rewind of the REAL rows rather than hand-inserted ones. The seeder writes
// names the keys do not imply (`manager` is "Team Lead") and an admin
// role_assignment beside them, so rows built here would be a state no
// installation was ever in — and a column a future seeder adds would be missing
// from them silently. Removing a key from what bootstrap wrote keeps all of it.
func rewindTo(ctx context.Context, t *testing.T, e *apptest.AppEnv, objects []string) {
	t.Helper()
	for _, object := range objects {
		if _, err := e.Owner.Exec(ctx,
			`UPDATE role SET permissions = permissions #- ARRAY['objects', $1] WHERE is_system`,
			object); err != nil {
			t.Fatalf("rewinding %q out of the seeded documents: %v", object, err)
		}
	}
	seedControlRole(ctx, t, e)
}

// seedControlRole puts the missing-predicate detector in front of a replay.
func seedControlRole(ctx context.Context, t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	if _, err := e.Owner.Exec(ctx,
		`INSERT INTO role (key, name, is_system, permissions) VALUES ($1, $1, false, $2)
		 ON CONFLICT (key) DO UPDATE SET permissions = EXCLUDED.permissions`,
		controlRole, controlDocument); err != nil {
		t.Fatalf("seeding the control role: %v", err)
	}
}

// rewindToBaselineEra replaces each seeded role's permissions document with the
// one that role held at the baseline, leaving every other column as the real
// seeder wrote it — the same discipline as rewindTo, and for the same reason:
// rows built here would be a state no installation was ever in.
//
// RowsAffected is checked. An UPDATE that matches nothing is not an error in
// Postgres, so a role the pre-state names and the database does not would leave
// that role at its MODERN document while this function reported success — and
// the replay would then be proving convergence from a state it never reached.
func rewindToBaselineEra(ctx context.Context, t *testing.T, e *apptest.AppEnv, documents map[string]json.RawMessage) {
	t.Helper()
	for _, role := range sortedKeys(documents) {
		tag, err := e.Owner.Exec(ctx,
			`UPDATE role SET permissions = $2 WHERE key = $1 AND is_system`,
			role, []byte(documents[role]))
		if err != nil {
			t.Fatalf("seeding the baseline-era document for %q: %v", role, err)
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("seeding the baseline-era document for %q matched %d system role rows, want 1 — "+
				"the bootstrap did not create the role this pre-state describes, so the replay would "+
				"start from a mixture of eras rather than from the baseline",
				role, tag.RowsAffected())
		}
	}
	seedControlRole(ctx, t, e)
}

// permissionWrite is one migration's writes to role.permissions.
//
// `statements`, not the whole file: a backfill rides along with the migration
// that introduces its object, so the same file also CREATEs its tables. Replaying
// the file against a database already at head fails on the CREATE and says
// nothing about the grant. The schema half is the head-catalog gate's job.
type permissionWrite struct {
	name       string
	statements []string
	objects    []string
}

var (
	// A candidate migration: one that mentions the column at all.
	// DELIBERATELY WEAK — a filter, not the judgement. The strict pattern below
	// decides, and a candidate the strict pattern cannot read is a hard failure
	// rather than a skip, so a spelling nobody anticipated (`UPDATE role AS r`,
	// `UPDATE ONLY role`, an upsert) stops the gate instead of vanishing from it.
	//
	// UNBOUNDED, and the word `role` is deliberately not required. This filter
	// used to demand the two words within 400 characters of each other, which
	// broke the contract above in the one direction that cannot be noticed: a
	// migration whose UPDATE carried a long comment before its SET fell out of
	// the candidate set with no fatal, was never replayed, and the arm still
	// converged — because the write it skipped was the write that would have
	// diverged. Measured: a ~460-character gap gives candidate=false while the
	// strict pattern matches. A distance window in front of a census is the
	// shortcut CLAUDE.md rule 8 forbids by name, and nothing measured that this
	// one bought anything.
	mentionsRolePermissions = regexp.MustCompile(`(?i)\bpermissions\b`)
	// A statement that writes the column, however the write is spelled. MERGE and
	// a quoted schema qualifier are included because Postgres accepts them and
	// this pattern deciding "not a write" is how a real grant leaves the gate.
	rolePermissionWrite = regexp.MustCompile(
		`(?is)\b(?:UPDATE|INSERT\s+INTO|MERGE\s+INTO)\s+(?:ONLY\s+)?(?:"?public"?\.)?"?role"?\b[\s\S]*?\bpermissions\b`)
	// The object a write grants, in EITHER jsonb path spelling Postgres accepts:
	// the brace text literal `'{objects,deal}'` and the array form
	// `ARRAY['objects', 'deal']`. Only the first was recognised, which made a
	// write using the second invisible to the rewind — no object removed, so the
	// isolating arm replayed it against the already-seeded grant and passed
	// without testing it. The array form is not exotic: it is what rewindTo uses
	// one screen below.
	objectPath      = regexp.MustCompile(`'\{objects,([a-z_0-9]+)\}'`)
	objectArrayPath = regexp.MustCompile(`(?i)ARRAY\[\s*'objects'\s*,\s*'([a-z_0-9]+)'\s*\]`)
	// Every objects-path literal, whatever the name inside. Its DISTINCT names are
	// counted against objectPath's so a name that class misses is loud rather
	// than dropped — distinct on both sides, because one object is normally
	// granted by several statements in the same migration.
	anyObjectPath = regexp.MustCompile(`'\{objects,([^}]*)\}'`)
	// Every array-form objects path, whatever is inside, counted against the
	// strict one for the same reason: a name that class cannot read must be loud.
	// The whole tail, not just the next element: the brace form tells a deeper
	// path from a plain one by the comma inside its capture, and an array pattern
	// that stopped at the second element produced a comma-free name for
	// ARRAY['objects','deal','delete'] — which then failed as an unreadable name
	// instead of being logged as out of rewind scope. Both spellings have to
	// reach the same judgement or the asymmetry IS the defect.
	anyObjectArrayPath = regexp.MustCompile(`(?i)ARRAY\[\s*'objects'\s*,\s*([^\]]*)\]`)
)

// rolePermissionMigrations reads the EMBEDDED core namespace — the same bytes
// dbmigrate applies — rather than walking the directory, so a moved package or a
// renamed suffix cannot quietly narrow what this gate examines.
func rolePermissionMigrations(t *testing.T) []permissionWrite {
	t.Helper()
	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading the core migrations: %v", err)
	}
	var found []permissionWrite
	for _, migration := range core.Migrations {
		if !mentionsRolePermissions.MatchString(migration.UpSQL) {
			continue
		}
		name := migration.Version + "_" + migration.Name
		statements := rolePermissionStatements(migration.UpSQL)
		if len(statements) == 0 {
			// The baseline declares the table and seeds nothing into it, which is
			// the one candidate that legitimately carries no write.
			//
			// Judged per STATEMENT. As a substring search over the whole file this
			// hatch was satisfied by a comment, so any migration that merely
			// mentioned the phrase could carry a write in a spelling the strict
			// pattern missed and leave the gate silent instead of failing it.
			if onlyDeclaresPermissions(migration.UpSQL) {
				continue
			}
			t.Fatalf("%s mentions permissions but no statement matched the write pattern.\n"+
				"Teach the pattern the spelling it uses — do NOT let the gate go quiet, because a "+
				"write it cannot see is a grant nobody checks.", name)
		}
		body := strings.Join(statements, "\n")
		objects := dedupe(append(
			objectPath.FindAllStringSubmatch(body, -1),
			objectArrayPath.FindAllStringSubmatch(body, -1)...))
		// Every objects-path literal, whatever its shape, must be accounted for.
		// A path naming a VERB (`{objects,deal,delete}`) is a legitimate write
		// this rewind cannot undo — removing the whole object would overshoot —
		// so it is reported as out of scope rather than as a name the pattern
		// failed to read. Fatalling on it refused the verb-widening backfill that
		// assertPreStateIsNotAlreadyTheAnswer calls the likelier next one.
		declared := dedupe(append(
			anyObjectPath.FindAllStringSubmatch(body, -1),
			normalizeArrayPaths(anyObjectArrayPath.FindAllStringSubmatch(body, -1))...))
		for _, m := range declared {
			if slices.Contains(objects, m) {
				continue
			}
			if strings.Contains(m, ",") {
				t.Logf("%s writes the deeper path {objects,%s}: the composed arm replays it, and the "+
					"per-write rewind leaves the object in place because removing it would undo more "+
					"than the write does", name, m)
				continue
			}
			t.Fatalf("%s names object %q, which the object pattern could not read; an object left in "+
				"place by the rewind makes the write a no-op and the comparison pass for the wrong "+
				"reason", name, m)
		}
		found = append(found, permissionWrite{name: name, statements: statements, objects: objects})
	}
	// NOT re-sorted. migrations.Core() returns dbmigrate.Load's output, which is
	// already ordered by VERSION — the order a real upgrade applies. Sorting here
	// by `version + "_" + name` looked like the same key and is not: '_' (0x5F)
	// outranks every digit, so whenever one version is a prefix of another
	// (178744982 beside 1787449829) the two orders invert, and mixed version
	// widths are exactly what this namespace ships. One invariant, one writer.
	return found
}

// normalizeArrayPaths rewrites an array-form path tail into the shape a brace
// capture has — `'deal', 'delete'` becomes `deal,delete` — so one comma test
// answers "is this a deeper path" for both spellings.
func normalizeArrayPaths(matches [][]string) [][]string {
	out := make([][]string, 0, len(matches))
	for _, m := range matches {
		tail := strings.NewReplacer("'", "", " ", "", "\t", "", "\n", "").Replace(m[1])
		out = append(out, []string{m[0], tail})
	}
	return out
}

// onlyDeclaresPermissions reports whether every statement in this migration that
// mentions the column merely DECLARES it — the baseline creating the table.
func onlyDeclaresPermissions(sql string) bool {
	sawDeclaration := false
	for _, statement := range splitStatements(sql) {
		if !strings.Contains(strings.ToLower(statement), "permissions") {
			continue
		}
		trimmed := strings.ToUpper(strings.TrimSpace(statement))
		if !strings.HasPrefix(trimmed, "CREATE TABLE") && !strings.HasPrefix(trimmed, "ALTER TABLE") {
			return false
		}
		sawDeclaration = true
	}
	return sawDeclaration
}

// rolePermissionStatements returns the statements in one migration that write
// role.permissions, and nothing else it does.
func rolePermissionStatements(sql string) []string {
	var out []string
	for _, statement := range splitStatements(sql) {
		if rolePermissionWrite.MatchString(statement) {
			out = append(out, statement)
		}
	}
	return out
}

// splitStatements cuts SQL on top-level semicolons.
//
// Quote-, dollar-quote- and block-comment aware, because a semicolon inside any
// of them is not a statement boundary. That matters more than it looks: pgx runs
// an argument-less Exec through the simple query protocol, which accepts several
// statements at once, so a wrong split can EXECUTE and leave this gate reporting
// green over a boundary it got wrong rather than failing loudly.
func splitStatements(sql string) []string {
	var out []string
	var current strings.Builder
	var quoted bool
	var tag string // the active $tag$ delimiter, empty when not dollar-quoted
	var block int  // /* */ nesting depth; Postgres allows nesting

	for i := 0; i < len(sql); i++ {
		rest := sql[i:]
		switch {
		case tag != "":
			if strings.HasPrefix(rest, tag) {
				current.WriteString(tag)
				i += len(tag) - 1
				tag = ""
				continue
			}
		case block > 0:
			if strings.HasPrefix(rest, "*/") {
				block--
				current.WriteString("*/")
				i++
				continue
			}
			if strings.HasPrefix(rest, "/*") {
				block++
				current.WriteString("/*")
				i++
				continue
			}
		case quoted:
			switch {
			case sql[i] == '\\' && i+1 < len(sql):
				// E'…\'…': the backslash escapes the next byte, and consuming it
				// here is what stops a quote-parity flip.
				current.WriteString(sql[i : i+2])
				i++
				continue
			case sql[i] == '\'':
				if i+1 < len(sql) && sql[i+1] == '\'' {
					current.WriteString("''")
					i++
					continue
				}
				quoted = false
			}
		case sql[i] == '\'':
			quoted = true
		case strings.HasPrefix(rest, "/*"):
			block++
			current.WriteString("/*")
			i++
			continue
		case strings.HasPrefix(rest, "--"):
			end := strings.IndexByte(rest, '\n')
			if end < 0 {
				i = len(sql)
				continue
			}
			i += end
			current.WriteByte('\n')
			continue
		case sql[i] == '$':
			if open := dollarTag.FindString(rest); open != "" {
				tag = open
				current.WriteString(open)
				i += len(open) - 1
				continue
			}
		case sql[i] == ';':
			if trimmed := strings.TrimSpace(current.String()); trimmed != "" {
				out = append(out, trimmed+";")
			}
			current.Reset()
			continue
		}
		current.WriteByte(sql[i])
	}
	if trimmed := strings.TrimSpace(current.String()); trimmed != "" {
		out = append(out, trimmed)
	}
	return out
}

// dollarTag matches an opening $$ or $name$ delimiter at the cursor.
var dollarTag = regexp.MustCompile(`^\$[a-zA-Z_0-9]*\$`)

// bootstrapInstallation provisions the installation these tests read.
//
// apptest.SetupApp resets the database and provisions nothing — `role`,
// `role_assignment` and `workspace` are all emptied by testdb.Reset — so this is
// the only thing that puts a seeded matrix in front of the assertions.
func bootstrapInstallation(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	ctx := context.Background()
	pwFile := filepath.Join(t.TempDir(), "admin-password")
	if err := os.WriteFile(pwFile, []byte("a-long-enough-password"), 0o600); err != nil {
		t.Fatalf("writing the bootstrap password: %v", err)
	}
	cfg, err := deployconfig.Parse([]byte(`version: 1
organization:
  name: RBAC Seed Parity
bootstrap_admin:
  email: admin@rbacparity.test
  display_name: Parity Admin
  password_file: ` + pwFile + `
`))
	if err != nil {
		t.Fatalf("parsing the deployment file: %v", err)
	}
	if err := compose.EnsureInstallation(ctx, e.Pool, slog.New(slog.DiscardHandler), cfg); err != nil {
		t.Fatalf("bootstrapping: %v", err)
	}
}

// readBaselineEraDefaults reads the committed baseline-era matrix.
//
// A plain file read: the pin that makes this file trustworthy is
// rbacbaselineerafixture_test.go in the unit lane, which has the full history
// this lane's shards do not.
func readBaselineEraDefaults(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(baselineEraDefaults)
	if err != nil {
		t.Fatalf("reading %s: %v — backend/gates/rbacbaselineerafixture_test.go names the command that "+
			"regenerates it from history", baselineEraDefaults, err)
	}
	// RawMessage, so the bytes that reach the database are the bytes the unit
	// lane pinned to history. Decoding into roleDocument and re-marshalling
	// dropped every key that struct does not model — policy.Document already
	// declares a third, `field_masks`, a privacy control — so the row seeded here
	// would have been narrower than the fixture while the comment claimed it was
	// what an installation "really held". A smaller input reaching the same PASS
	// is the one failure this family must not have.
	var documents map[string]json.RawMessage
	if err := json.Unmarshal(raw, &documents); err != nil {
		t.Fatalf("decoding %s: %v", baselineEraDefaults, err)
	}
	if len(documents) == 0 {
		t.Fatalf("%s holds no roles; an empty pre-state would seed nothing and leave the replay "+
			"running against today's documents", baselineEraDefaults)
	}
	return documents
}

// readMatrixAsValues decodes a matrix file into plain values, which is what the
// distance check below compares.
//
// A DECLARED MIRROR of decodeMatrix in backend/gates/rbacbaselineerafixture_test.go:
// the same question is asked in the unit lane, where it costs a second, and
// again here, where the arm must not trust a file it merely reads. Both use
// decoded values rather than the raw bytes so indentation is not distance, and
// both compare in BOTH directions.
func readMatrixAsValues(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var matrix map[string]any
	if err := json.Unmarshal(raw, &matrix); err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	if len(matrix) == 0 {
		t.Fatalf("%s holds no roles; an empty matrix makes every comparison against it pass", path)
	}
	return matrix
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupe(matches [][]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}
