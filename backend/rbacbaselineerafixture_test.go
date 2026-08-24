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
	"reflect"
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
	baselineEraSource  = "backend/migrations/testdata/rbac_seeded_defaults.json"
	baselineEraFixture = "migrations/testdata/rbac_baseline_era_defaults.json"
)

func TestBaselineEraFixtureIsTheMatrixTheBaselineSeeded(t *testing.T) {
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

// The pre-state must still be a pre-state: strictly smaller than today's matrix.
//
// Not defensive tidiness. The day the vocabulary stops growing, the baseline-era
// documents catch up with head, every backfill replayed over them no-ops, and the
// composition arm reports PASS having compared a state with itself. The
// integration arm refuses that too, but it refuses it 40 seconds into a database
// lane; here it is a second, and this is the lane that runs on every push.
func TestBaselineEraFixtureIsStillBehindTheSeededMatrix(t *testing.T) {
	before := readJSONFixture(t, baselineEraFixture)
	after := readJSONFixture(t, "migrations/testdata/rbac_seeded_defaults.json")

	if reflect.DeepEqual(before, after) {
		t.Fatalf("%s already equals the matrix it is supposed to predate, so the composition gate "+
			"has no distance to prove anything over: every backfill can no-op and the comparison "+
			"still passes.\nRepoint baselineEraCommit at a later consolidation floor if one has "+
			"landed, or retire the arm — do NOT leave it green over a comparison with nothing in it.",
			baselineEraFixture)
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
	// -C .. : these tests run from backend/, and the paths recorded in history
	// are relative to the repository root.
	out, err := exec.Command("git", "-C", "..", "show",
		baselineEraCommit+":"+baselineEraSource).Output()
	if err != nil {
		detail := err.Error()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		t.Fatalf("reading %s:%s: %s\nThis gate derives the composition arm's pre-state from history, "+
			"so it needs FULL history. If this is CI, the job running the backend unit lane checks "+
			"out shallow — give it `fetch-depth: 0`, as integration-unit-coverage already has. "+
			"Deepen the checkout; do not weaken the gate.",
			baselineEraCommit, baselineEraSource, detail)
	}
	return decodeMatrix(t, out, baselineEraCommit+":"+baselineEraSource)
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
