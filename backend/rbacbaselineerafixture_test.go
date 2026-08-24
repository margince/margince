// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package backendarch

// The pre-state the RBAC composition gate replays over must not be hand-editable.
//
// compose/integration/rbacseedparity_integration_test.go replays every
// role.permissions backfill over the documents an installation bootstrapped at
// the migration baseline actually held, and asserts it converges on today's
// seeded matrix. That arm is the only one that can see an object added to the
// policy with NO backfill written for it — the sibling arm derives its pre-state
// from the migrations themselves, so an object no migration mentions is never
// absent from it.
//
// Which makes the pre-state the thing an unwilling author would edit. Move the
// object into the starting state and the convergence the backfill never delivered
// is already there, with nothing to report it: the object names would not have
// changed anywhere else.
//
// So the fixture is DERIVED, and this is where the derivation is checked. It is
// `git show <baseline>:<the seeded-matrix fixture>` — the same file, pinned to
// policy by identity's bridge test on that day exactly as it is today, so
// recovering it needs no evaluator. It already IS what the server seeded then.
//
// WHY THE CHECK LIVES HERE AND THE READER LIVES THERE. Reading history needs a
// full checkout, and only the backend unit lane has one: _lane-integration.yml
// gives integration-unit-coverage `fetch-depth: 0`, while the integration SHARDS
// check out shallow. So the gate that needs history runs in the lane that has it,
// and the integration arm reads a plain committed file — which is the same split
// rbac_seeded_defaults.json already uses, identity rendering it and
// compose/integration reading it.

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const (
	// baselineEraCommit is the commit that replaced core's 318 migrations with
	// one baseline (#2189). It is the FLOOR of the upgrade path rather than
	// merely an old commit: dbmigrate.assertLedgerMatches refuses any database
	// whose ledger records core 0001 under a different name, and every
	// pre-baseline database records it as "foundation" — such a database cannot
	// be repaired forward, and the migrator says so. An installation
	// bootstrapped here is therefore the OLDEST one that can reach head.
	baselineEraCommit = "0e4806a38"

	// baselineEraSource is where the seeded matrix lived at that commit, and
	// baselineEraFixture is the committed copy this gate pins to it.
	seededDefaultsPath = "migrations/testdata/rbac_seeded_defaults.json"
	baselineEraSource  = "backend/" + seededDefaultsPath
	baselineEraFixture = "migrations/testdata/rbac_baseline_era_defaults.json"
)

func TestBaselineEraFixtureIsTheMatrixTheBaselineSeeded(t *testing.T) {
	assertCommitIsTheUpgradeFloor(t)
	derived := baselineEraFromHistory(t)
	committed := readJSONFixture(t, baselineEraFixture)

	if !reflect.DeepEqual(derived, committed) {
		t.Errorf("%s does not match %s:%s.\n"+
			"This fixture is the state an installation bootstrapped at the baseline really held, and "+
			"the composition gate replays the backfills over it. An object may not be moved into it "+
			"to excuse writing that object's backfill migration — that is precisely the edit this "+
			"gate exists to refuse.\n"+
			"If the difference is legitimate, regenerate rather than hand-edit:\n"+
			"  git show %s:%s > backend/%s",
			baselineEraFixture, baselineEraCommit, baselineEraSource,
			baselineEraCommit, baselineEraSource, baselineEraFixture)
	}
}

// The pre-state must still DIFFER from today's matrix.
//
// Differs, and deliberately not "is strictly smaller": a backfill that widens an
// existing object's verbs changes no key and no count, and that is real distance
// for the replay to cross. Demanding a smaller document would refuse a legitimate
// grant-value backfill, so inequality is both the weakest honest bar and the
// right one.
//
// Not defensive tidiness. The day the vocabulary stops growing, the baseline-era
// documents catch up with head, every backfill replayed over them no-ops, and the
// composition arm reports PASS having compared a state with itself. The
// integration arm refuses that too, but only later, in the database lane; here it
// is cheap, and this is the lane that runs on every push.
func TestBaselineEraFixtureStillDiffersFromTheSeededMatrix(t *testing.T) {
	before := readJSONFixture(t, baselineEraFixture)
	after := readJSONFixture(t, seededDefaultsPath)

	if reflect.DeepEqual(before, after) {
		t.Fatalf("%s already equals the matrix it is supposed to predate, so the composition gate "+
			"has no distance to prove anything over: every backfill can no-op and the comparison "+
			"still passes.\nRepoint baselineEraCommit at a later consolidation floor if one has "+
			"landed (and regenerate with the command in the sibling test's message), or retire the "+
			"arm — do NOT leave it green over a comparison with nothing in it.",
			baselineEraFixture)
	}
}

// assertCommitIsTheUpgradeFloor proves baselineEraCommit is the consolidation
// floor, rather than merely a commit somebody named.
//
// WITHOUT THIS THE PIN IS REPOINTABLE, and the design it replaced was stronger on
// exactly this point: the deleted cohort gate read a path
// (crm-auth/internal/policy/policy.go) that no longer exists in a modern tree, so
// moving the constant forward failed loudly. The path this gate reads exists at
// every commit from the baseline onward, including commits on the author's own
// branch, so repointing would otherwise succeed in silence — and that is a
// working defeat, not a hypothetical one:
//
//	commit A adds an object to coreObjects and regenerates the seeded matrix,
//	        with NO backfill migration;
//	commit B repoints baselineEraCommit at A and regenerates this fixture from it.
//
// Both gates here pass (the fixture IS `git show A:…`), the distance check passes
// on any unrelated difference, and the composed arm passes because the object is
// already in the pre-state. The object then 403s forever on every upgraded
// installation — the precise defect this family exists to catch.
//
// The floor is derivable because the consolidation is what made core's history one
// file: at the floor, core holds exactly the 0001 baseline pair, and every later
// commit carries additional migrations.
func assertCommitIsTheUpgradeFloor(t *testing.T) {
	t.Helper()
	listing := gitAtBaselineEra(t, "ls-tree", "--name-only", baselineEraCommit, "backend/migrations/core/")
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, filepath.Base(name))
		}
	}
	slices.Sort(names)
	want := []string{"0001_baseline.down.sql", "0001_baseline.up.sql"}
	if !slices.Equal(names, want) {
		// The COUNT and a sample, never the whole listing: at a later commit this
		// is sixty filenames, and a failure nobody can read is a failure nobody
		// acts on.
		sample := names
		if len(sample) > 4 {
			sample = sample[:4]
		}
		t.Fatalf("%s is not the consolidation floor: its backend/migrations/core/ holds %d file(s) "+
			"(%v…), want exactly %v.\n"+
			"This constant must name the commit that replaced core's history with one baseline, because "+
			"the pre-state it produces is only \"what the oldest reachable installation held\" at THAT "+
			"commit. Any later commit already carries backfills, so pointing here instead would fold an "+
			"object's grant into the starting state and hide the missing migration for it — which is the "+
			"defect this whole family exists to catch.",
			baselineEraCommit, len(names), sample, want)
	}
}

// baselineEraFromHistory reads the seeded matrix as it stood at the baseline.
//
// It FAILS rather than skipping when history is unreachable. The backend unit
// lane checks out at fetch-depth: 0 today; if that ever changes the honest
// outcome is a red gate naming the cause, because a gate that degraded to "no
// history, nothing to compare" would look exactly like a passing one — and what
// it stops being able to see is a fixture edited to excuse a missing backfill.
func baselineEraFromHistory(t *testing.T) map[string]any {
	t.Helper()
	raw := gitAtBaselineEra(t, "show", baselineEraCommit+":"+baselineEraSource)
	return decodeMatrix(t, []byte(raw), baselineEraCommit+":"+baselineEraSource)
}

// gitAtBaselineEra runs one read-only git command against the repository root.
//
// It FAILS rather than skipping when history is unreachable. Both lanes that run
// this gate check out at fetch-depth: 0 today; if that changes, the honest
// outcome is a red gate naming the cause, because a gate that degraded to "no
// history, nothing to compare" would look exactly like a passing one — and what
// it stops being able to see is a pre-state edited or repointed to excuse a
// missing backfill.
//
// Every argument is a compile-time constant: nothing from a fixture, an
// environment variable or a test name reaches the argv.
func gitAtBaselineEra(t *testing.T, args ...string) string {
	t.Helper()
	// -C .. : these tests run from backend/, and the paths recorded in history
	// are relative to the repository root.
	out, err := exec.Command("git", append([]string{"-C", ".."}, args...)...).Output()
	if err != nil {
		detail := err.Error()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		t.Fatalf("git %s: %s\nThis gate derives the composition arm's pre-state from history, so it "+
			"needs FULL history. If this is CI, the job running the backend unit lane checks out "+
			"shallow — give it `fetch-depth: 0`, as integration-unit-coverage already has. Deepen "+
			"the checkout; do not weaken the gate.", strings.Join(args, " "), detail)
	}
	return string(out)
}

func readJSONFixture(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return decodeMatrix(t, raw, path)
}

// decodeMatrix decodes a role matrix into plain values, so key order and
// indentation do not read as a difference while a changed TYPE still does —
// `{"read": true}` and `{"read": "true"}` must not compare equal.
func decodeMatrix(t *testing.T, raw []byte, source string) map[string]any {
	t.Helper()
	var matrix map[string]any
	if err := json.Unmarshal(raw, &matrix); err != nil {
		t.Fatalf("decoding %s: %v", source, err)
	}
	if len(matrix) == 0 {
		t.Fatalf("%s holds no roles; an empty matrix would make every comparison against it pass", source)
	}
	return matrix
}
