// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// One set of retention actions, spelled in three places.
//
// `retention_policy.action` carries a CHECK; `crm.yaml` publishes
// RetentionAction to the API; `public-events.yaml` publishes the same values on
// retention.applied to subscribers. All three are hand-written, and each is the
// authority for a different reader — the database refuses a row, the API refuses
// a request, and the event contract is what lets a subscriber switch on the
// value exhaustively.
//
// They fail differently in each direction, which is why this asserts EQUALITY
// rather than containment:
//
//   - a value in the database and not on the event is one an emit site can ship
//     and every subscriber drops in silence, which reads exactly like no event
//     having been emitted;
//   - a value on the event and not in the database is a promise to subscribers
//     that nothing can produce, so a consumer writes a branch that never runs;
//   - a value in the API and not the database is a request accepted and then
//     refused by a constraint, with the caller told nothing useful.
//
// The gate is what makes the third copy safe to have. Deriving one from another
// was the alternative, and there is nowhere to derive from: the CHECK is SQL in
// a shipped migration, and the two contracts are read by generators that resolve
// no cross-document reference.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// retentionActionCheck reads the values the database admits, out of the schema
// head rather than any one migration: a later migration may have widened or
// narrowed the CHECK, and the head is what a running installation has.
func retentionActionCheck(t *testing.T) []string {
	t.Helper()
	const catalog = "migrations/testdata/head_catalog.txt"
	const constraint = "public.retention_policy.retention_policy_action_check "
	raw, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatalf("reading the schema head: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, constraint) {
			continue
		}
		values := quotedSQLText.FindAllStringSubmatch(line, -1)
		if len(values) == 0 {
			t.Fatalf("the retention action CHECK reads %q and names no value — this gate would compare against nothing", line)
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			// SQL doubles a quote to escape it; the stored value has one.
			out = append(out, strings.ReplaceAll(value[1], "''", "'"))
		}
		// EVERY literal, or none of them. A value the pattern cannot read is
		// dropped from the set and the remaining ones still match the
		// contract's — so the gate would pass while the two had diverged by
		// exactly the value nobody could parse, which is the failure a census
		// must not have. Removing what was read must leave no quote behind.
		if rest := quotedSQLText.ReplaceAllString(line, ""); strings.Contains(rest, "'") {
			t.Fatalf("the retention action CHECK carries a quoted value this reader cannot parse:\n\t%s\n"+
				"It read %v and left %q. A value it cannot see is one it cannot compare, and the values "+
				"it CAN see would still match the contract — so this gate would report agreement over a divergence",
				line, out, rest)
		}
		sort.Strings(out)
		return out
	}
	t.Fatalf("no retention_policy action CHECK at schema head — either the column stopped being constrained, which is its own defect, or this reader stopped finding it")
	return nil
}

// quotedSQLText pulls the literals out of a rendered CHECK, which the catalog
// writes as `ARRAY['archive'::text, …]`.
// quotedSQLText matches one SQL string literal and its cast, doubled quotes
// included. Deliberately not `[a-z_]+`: a value carrying a hyphen, a capital or
// a space is one this reader would silently skip, and skipping is the direction
// that reports agreement over a divergence.
var quotedSQLText = regexp.MustCompile(`'((?:''|[^'])*)'::text`)

// TestTheRetentionActionSetIsOneSet holds the three spellings together.
func TestTheRetentionActionSetIsOneSet(t *testing.T) {
	t.Parallel()
	database := retentionActionCheck(t)
	for _, spelling := range []struct {
		what     string
		path     string
		schema   string
		property string
	}{
		{"the API's RetentionAction", "api/crm.yaml", "RetentionAction", ""},
		{"retention.applied's action", "api/public-events.yaml", "PublicEventRetentionApplied", "action"},
	} {
		doc := loadOpenAPIDocument(t, spelling.path)
		published := contractEnum(t, doc, spelling.schema, spelling.property)
		if len(published) == 0 {
			t.Errorf("%s declares no enum — an open field is what this gate exists to find", spelling.what)
			continue
		}
		names := make([]string, 0, len(published))
		for value := range published {
			names = append(names, value)
		}
		sort.Strings(names)
		if strings.Join(names, ",") != strings.Join(database, ",") {
			t.Errorf("%s publishes %v and retention_policy.action admits %v — one set, and the two ends of it disagree",
				spelling.what, names, database)
		}
	}
}
