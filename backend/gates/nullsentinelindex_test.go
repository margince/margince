// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A nullable column in a unique key is held by NULLS NOT DISTINCT rather than
// by a sentinel.
//
// A NULL is distinct from every other NULL, so a natural key with a nullable
// column does not constrain the rows where that column is null. COALESCEing to a
// chosen value closes that — and makes the chosen value unsayable: a row that
// genuinely stores it collides with the null row and is refused with a uniqueness
// error naming neither cause. The nil UUID is the worst pick available, being the
// value a caller sends when it means "unset" and the one the contract calls
// forbidden as a placeholder, and '' cannot tell "no company" from a company
// whose name is empty.
//
// Postgres has had NULLS NOT DISTINCT since 15. It says what it means, leaves
// every value sayable, and indexes the column rather than an expression over it.
//
// Coalescing SEVERAL REAL COLUMNS is a different thing and not what this reads:
// COALESCE(contact_id, company_id, deal_id) means "whichever one is set", which
// NULLS NOT DISTINCT cannot express. Such an index carries no literal, so it does
// not match the detector rather than needing to be excused by name.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// sentinelCoalesce returns the first COALESCE in a column list that defaults to a
// quoted literal. An argument may itself be parenthesised — (scope_id)::text is
// one — so parenGroup reads the arguments by balance, which a character class
// cannot. coalesceCall is shared with TestTheIdleBaseIsSpelledOnce, whose comment records
// the two spellings a pattern over the arguments went blind on; this tree writes
// the keyword in both cases, linkedin_connection in lowercase.
//
// The search continues INSIDE a call that does not match, because a sentinel can
// sit nested in one that is not itself a sentinel.
func sentinelCoalesce(columns string) string {
	for at := 0; at < len(columns); {
		opens := coalesceCall.FindStringIndex(columns[at:])
		if opens == nil {
			return ""
		}
		call := at + opens[0]
		arguments, _, ok := parenGroup(columns[call:])
		if !ok {
			return ""
		}
		if defaultsToALiteral(arguments) {
			return columns[call : call+strings.Index(columns[call:], arguments)+len(arguments)+1]
		}
		// Past the keyword only, not past the call: a nested sentinel is still ahead.
		at = at + opens[1]
	}
	return ""
}

// defaultsToALiteral reports whether any argument past the first is a quoted
// literal. The first is the column being defaulted; a literal after it is the
// sentinel standing in for that column's null.
func defaultsToALiteral(arguments string) bool {
	for _, argument := range splitTopLevel(arguments)[1:] {
		if strings.HasPrefix(argument, "'") {
			return true
		}
	}
	return false
}

var uniqueIndexDefinition = regexp.MustCompile(
	`^(public|ext)\.([A-Za-z0-9_]+) CREATE UNIQUE INDEX \S+ ON (?:public|ext)\.([a-z0-9_]+) USING btree (.*)$`)

// sentinelsThatAreNotNullMarkers are the literals this schema keeps, each because
// the COALESCE does something other than stand in for a null in the key.
var sentinelsThatAreNotNullMarkers = gatekit.Waive(map[string]string{
	"scheduled_send_one_held_message_per_seat": "" +
		"its two literals default a jsonb payload INSIDE md5(), and md5 of anything is never null, so " +
		"NULLS NOT DISTINCT has nothing to do for those columns — the default belongs in the expression " +
		"whose digest the key compares. The three plain sentinels it used to carry are gone",
	"uq_activity_participant_identity": "" +
		"its four literals stand inside md5(), whose digest the key compares. Each is the empty text, and " +
		"no stored value yields it: a uuid never prints empty, and a text prints its length first. " +
		"TestAParticipantKeyTellsEveryIdentityApart holds a null apart from an empty text",
})

// sentinelIndexesBeingRetired lists old indexes kept for one release of a
// two-step key swap (recipe: docs/how-to/apply-migrations.md). A key is
// "<schema>.<table>.<index>", and the reason names the release that drops it.
// An entry whose index is gone, or whose table lacks the replacement, fails.
var sentinelIndexesBeingRetired = gatekit.Waive(map[string]string{})

// uniqueIndexFloor is what the catalog held when this gate was written. A census
// that silently reads a smaller schema reports PASS with nothing to notice.
const uniqueIndexFloor = 400

func TestNoUniqueKeyFakesANullWithASentinel(t *testing.T) {
	t.Parallel()
	defer sentinelsThatAreNotNullMarkers.AssertAllMatched(t)
	defer sentinelIndexesBeingRetired.AssertAllMatched(t)

	findings, examined := sentinelIndexFindings(t, catalogRecords(t), sentinelIndexesBeingRetired)
	for _, finding := range findings {
		t.Error(finding)
	}
	if examined < uniqueIndexFloor {
		t.Fatalf("read %d unique indexes, fewer than the %d this gate was written against — "+
			"the census is reading less than the schema", examined, uniqueIndexFloor)
	}
}

// sentinelIndexFindings returns one message per unique index that keys off a
// sentinel without a kept exception or a declared swap, and the count read.
func sentinelIndexFindings(t testing.TB, records []string, retiring *gatekit.Waivers[string]) ([]string, int) {
	t.Helper()
	var findings []string
	examined := 0
	for _, record := range records {
		m := uniqueIndexDefinition.FindStringSubmatch(strings.TrimSpace(record))
		if m == nil {
			continue
		}
		examined++
		schema, index, table, definition := m[1], m[2], m[3], m[4]
		// A key's own COALESCE lives in the column list; a predicate's belongs to
		// the WHERE and binds which rows are indexed, not how they are compared.
		columns, rest, ok := parenGroup(definition)
		if !ok {
			findings = append(findings, fmt.Sprintf("%s on %s: cannot read its column list from %q", index, table, definition))
			continue
		}
		found := sentinelCoalesce(columns)
		if found == "" || sentinelsThatAreNotNullMarkers.Waived(t, index) {
			continue
		}
		if !retiring.Waived(t, schema+"."+table+"."+index) {
			findings = append(findings, fmt.Sprintf("%s on %s keys off %s, so the sentinel is a value no row may store — "+
				"drop the COALESCE and append NULLS NOT DISTINCT to the index instead; if the old index must stay for one "+
				"release while the new one is added, declare it in sentinelIndexesBeingRetired as %s.%s.%s",
				index, table, found, schema, table, index))
			continue
		}
		if !hasNullsNotDistinctReplacement(records, schema, table, index, columns, rest) {
			findings = append(findings, fmt.Sprintf("%s on %s is declared as a swap in progress, but %s has no other unique "+
				"index over the same key columns that is NULLS NOT DISTINCT; add the replacement in the same release",
				index, table, table))
		}
	}
	return findings, examined
}

// keyColumns reduces a key's column list to what names the key: each sentinel
// COALESCE becomes its first argument, and casts and parentheses are dropped.
func keyColumns(columns string) []string {
	var out []string
	for _, column := range splitTopLevel(columns) {
		column = strings.TrimSpace(column)
		if coalesceCall.MatchString(column) {
			if inner, _, ok := parenGroup(column); ok {
				column = splitTopLevel(inner)[0]
			}
		}
		column = castSuffix.ReplaceAllString(column, "")
		out = append(out, strings.NewReplacer("(", "", ")", "", " ", "").Replace(column))
	}
	return out
}

var castSuffix = regexp.MustCompile(`::[a-z_ ]+`)

// hasNullsNotDistinctReplacement reports whether another unique index on the
// same table covers the old key's columns and predicate with NULLS NOT DISTINCT.
func hasNullsNotDistinctReplacement(records []string, schema, table, oldIndex, oldColumns, oldRest string) bool {
	wantColumns := strings.Join(keyColumns(oldColumns), ",")
	wantRest := strings.TrimSpace(oldRest)
	for _, record := range records {
		m := uniqueIndexDefinition.FindStringSubmatch(strings.TrimSpace(record))
		if m == nil || m[1] != schema || m[3] != table || m[2] == oldIndex {
			continue
		}
		columns, rest, ok := parenGroup(m[4])
		if !ok || !strings.Contains(rest, "NULLS NOT DISTINCT") || sentinelCoalesce(columns) != "" {
			continue
		}
		rest = strings.TrimSpace(strings.Replace(rest, "NULLS NOT DISTINCT", "", 1))
		if strings.Join(keyColumns(columns), ",") == wantColumns && strings.Join(strings.Fields(rest), " ") == strings.Join(strings.Fields(wantRest), " ") {
			return true
		}
	}
	return false
}

// TestASwapInProgressIsToleratedOnlyWhenDeclared: the old index is a finding
// without a declaration. With one and a replacement it is silent. Without the
// replacement it is a finding again.
func TestASwapInProgressIsToleratedOnlyWhenDeclared(t *testing.T) {
	t.Parallel()
	oldIndex := "public.uq_old CREATE UNIQUE INDEX uq_old ON public.mail_thread USING btree (workspace_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::uuid))"
	newIndex := "public.uq_new CREATE UNIQUE INDEX uq_new ON public.mail_thread USING btree (workspace_id, team_id) NULLS NOT DISTINCT"
	declare := func() *gatekit.Waivers[string] {
		return gatekit.Waive(map[string]string{
			"public.mail_thread.uq_old": "old key of the team swap; the next release drops it once no binary infers it",
		})
	}
	undeclared, examined := sentinelIndexFindings(t, []string{oldIndex, newIndex}, gatekit.Waive(map[string]string{}))
	if examined != 2 || len(undeclared) != 1 || !strings.Contains(undeclared[0], "uq_old") {
		t.Fatalf("without a declaration the old index must be the one finding, got %d of %d: %v", len(undeclared), examined, undeclared)
	}
	if findings, _ := sentinelIndexFindings(t, []string{oldIndex, newIndex}, declare()); len(findings) != 0 {
		t.Fatalf("a declared swap with its replacement must not be reported, got %v", findings)
	}
	noReplacement, _ := sentinelIndexFindings(t, []string{oldIndex}, declare())
	if len(noReplacement) != 1 || !strings.Contains(noReplacement[0], "no other unique index") {
		t.Fatalf("a declared swap without a replacement must be a finding, got %v", noReplacement)
	}
	otherKey := strings.Replace(newIndex, "team_id", "owner_id", 1)
	if findings, _ := sentinelIndexFindings(t, []string{oldIndex, otherKey}, declare()); len(findings) != 1 {
		t.Fatalf("a replacement over other columns must not count, got %v", findings)
	}
}

// TestTheSentinelDetectorFiresOnWhatItIsFor plants the shapes the census must
// tell apart. Without it a detector that matched nothing would pass.
func TestTheSentinelDetectorFiresOnWhatItIsFor(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name     string
		columns  string
		sentinel bool
	}{
		{"the nil uuid", `role_id, user_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::uuid)`, true},
		{"an empty string", `owner_user_id, COALESCE(normalized_company, ''::text)`, true},
		{"an epoch date", `owner_user_id, COALESCE(connected_on, '1970-01-01'::date)`, true},
		{"a word standing for a scope", `metric, COALESCE((scope_id)::text, 'workspace'::text)`, true},
		{"whichever column is set", `activity_id, COALESCE(contact_id, company_id, deal_id, lead_id, project_id)`, false},
		{"two real columns", `run_id, COALESCE(target_row_id, contact_id)`, false},
		{"no coalesce at all", `role_id, user_id, team_id`, false},
		// The spellings Postgres folds and this tree uses. linkedin_connection's
		// own upsert was lowercase, so a detector reading one case would have
		// passed the very clause that broke.
		{"lowercase", `owner_user_id, coalesce(normalized_company, ''::text)`, true},
		{"a space before the parenthesis", `owner_user_id, COALESCE (connected_on, 'epoch'::date)`, true},
		// Nested: the outer call defaults to a column, so a search that skipped
		// past a non-matching call would never see the sentinel inside it.
		{"a sentinel inside a plainer call", `COALESCE(primary_at, COALESCE(seen_at, 'epoch'::date))`, true},
	} {
		if got := sentinelCoalesce(c.columns) != ""; got != c.sentinel {
			t.Errorf("%s: read as sentinel=%t, want %t — %s", c.name, got, c.sentinel, c.columns)
		}
	}
}

// TestAPredicatesCoalesceIsNotAKeysCoalesce holds the one distinction that makes
// the census readable: relationship's uq_rel_employment COALESCEs inside its
// WHERE, and nothing about its key is faked.
func TestAPredicatesCoalesceIsNotAKeysCoalesce(t *testing.T) {
	t.Parallel()

	definition := `(contact_id, company_id) WHERE ((kind = 'employment'::text) AND ` +
		`(COALESCE(ended_at, '9999-12-31'::date) > now()))`
	columns, _, ok := parenGroup(definition)
	if !ok {
		t.Fatalf("cannot read the column list of %q", definition)
	}
	if found := sentinelCoalesce(columns); found != "" {
		t.Errorf("a COALESCE in the predicate was read as a key sentinel: %q", found)
	}
}

// onConflictClause finds the inference list of an ON CONFLICT, which names the
// index Postgres must match. Only the clause's own parentheses are read: what
// follows in DO UPDATE is assignment, not inference.
var onConflictClause = regexp.MustCompile(`(?is)ON CONFLICT\s*\(`)

// goSourceFloor is what the tree held when this gate was written, for the reason
// uniqueIndexFloor exists.
const goSourceFloor = 2000

// TestNoUpsertInfersASentinelIndex is the other side of the same rule. An
// ON CONFLICT names its index by expression, so an upsert still spelling
// COALESCE(col, 'sentinel') stops matching the moment the index becomes
// NULLS NOT DISTINCT — and Postgres says so at runtime (42P10), not at build time.
// Converting an index without its upserts is how this change first broke every
// writer of two tables.
//
// An index coalescing only real columns is untouched by that, here as above: its
// clause carries no literal, so it does not match. A clause built from a
// constant is invisible to this, which is the one shape it cannot see.
func TestNoUpsertInfersASentinelIndex(t *testing.T) {
	t.Parallel()

	read := 0
	for _, path := range goSourceFiles(t, "..") {
		body, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		read++
		// The string literals, not the file: a census over raw bytes reads prose
		// about an inference as an inference, and go/parser is already the tool
		// this package uses to see only real SQL.
		//
		// Joined, because a statement is often built from several — consentproof
		// splices a column name mid-clause, so the parenthesis that opens an
		// inference list and the one that closes it sit in different literals. The
		// spliced expression itself is this census's blind spot: an identifier
		// cannot hide a sentinel, but a COALESCE held in a variable could.
		statements := strings.Join(sqlStringsIn(t, path, body), "\n")
		{
			for _, at := range onConflictClause.FindAllStringIndex(statements, -1) {
				inference, _, ok := parenGroup(statements[at[1]-1:])
				if !ok {
					t.Errorf("%s: cannot read the inference list of an ON CONFLICT in %q — "+
						"an unreadable clause is not a passing one", path, statements[at[0]:at[1]])
					continue
				}
				if found := sentinelCoalesce(inference); found != "" {
					t.Errorf("%s infers its index with %s — an index on that column is "+
						"NULLS NOT DISTINCT, so the clause matches nothing and the upsert "+
						"fails at runtime with 42P10", path, found)
				}
			}
		}
	}

	if read < goSourceFloor {
		t.Fatalf("read %d Go files, fewer than the %d this gate was written against — "+
			"the census is reading less than the tree", read, goSourceFloor)
	}
}
