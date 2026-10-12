// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// An upsert onto a partial unique index repeats that index's predicate.
//
// Postgres infers the index for ON CONFLICT by expression, and it matches a PARTIAL
// index only from a statement carrying the same WHERE. Omit it and there is nothing
// to infer: SQLSTATE 42P10, at runtime, on the first repeated write. Nothing in a
// build or a review sees it — the statement is valid SQL and the index is right
// there, apparently covering it.
//
// This gate exists because a census that read only Go missed it. Narrowing
// team_name_unique to the live rows left `scripts/seed-dev.sql` inferring on the
// shape that had gone, and the first thing to notice was CI booting a stack it could
// not seed. SQL lives in .sql files as well as in Go strings, so the corpus here is
// both.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	uniqueIndexHead = regexp.MustCompile(
		`(?i)^public\.\w+ CREATE UNIQUE INDEX (\w+) ON public\.(\w+) USING \w+ (\()`)
	insertTarget   = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(?:public\.)?(\w+)`)
	conflictTarget = regexp.MustCompile(`(?is)ON\s+CONFLICT\s*\(([^)]*)\)\s*(?:WHERE\s+(.+?))?\s*DO\s+(?:NOTHING|UPDATE)`)
)

// uniqueIndex is one index's shape, as the committed catalog states it.
type uniqueIndex struct {
	name      string
	columns   string
	predicate string // empty for an unconditional index
}

// uniqueIndexesByTable reads the committed schema, which is the authority on what
// an upsert can infer. Deriving the corpus from the catalog rather than a list here
// means a new partial index enrols its writers without this file changing.
func uniqueIndexesByTable(t *testing.T) map[string][]uniqueIndex {
	t.Helper()
	catalog := filepath.Join(moduleRoot(t), "migrations", "testdata", "head_catalog.txt")
	body, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	byTable := map[string][]uniqueIndex{}
	var declared, read int
	for line := range strings.SplitSeq(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(strings.ToUpper(line), "CREATE UNIQUE INDEX") {
			continue
		}
		declared++
		name, table, index, ok := parseUniqueIndex(line)
		if !ok {
			continue
		}
		read++
		index.name = name
		byTable[table] = append(byTable[table], index)
	}
	// The floor COUNTS, rather than asking whether anything was found at all. A
	// parser that reads most of the catalog and quietly drops the awkward shapes —
	// an expression key, a NULLS NOT DISTINCT — is the failure this gate exists to
	// prevent, committed by the gate itself: it reports PASS over a smaller schema
	// and there is no assertion to notice.
	if read != declared {
		t.Fatalf("the catalog declares %d unique indexes and this read %d: the %d it could "+
			"not parse are indexes whose upserts nothing here checks", declared, read, declared-read)
	}
	return byTable
}

// parseUniqueIndex reads one catalog line into the index it describes.
//
// By hand rather than by one regular expression, because the key list can hold
// parentheses of its own — `lower(name)`, `(expr)` — and a pattern that stops at the
// first `)` reads a truncated key list for ten of this tree's indexes and no key
// list at all for the rest of that line. Depth counting is what makes an expression
// key readable.
func parseUniqueIndex(line string) (name, table string, index uniqueIndex, ok bool) {
	head := uniqueIndexHead.FindStringSubmatchIndex(line)
	if head == nil {
		return "", "", uniqueIndex{}, false
	}
	name = line[head[2]:head[3]]
	table = line[head[4]:head[5]]
	keys, rest, ok := balancedGroup(line[head[6]:])
	if !ok {
		return "", "", uniqueIndex{}, false
	}
	// NULLS NOT DISTINCT sits between the key list and the predicate and says
	// nothing about either, so it is read past rather than matched into a position.
	rest = strings.TrimSpace(rest)
	if upper := strings.ToUpper(rest); strings.HasPrefix(upper, "NULLS NOT DISTINCT") {
		rest = strings.TrimSpace(rest[len("NULLS NOT DISTINCT"):])
	}
	index.columns = flattenColumns(keys)
	if upper := strings.ToUpper(rest); strings.HasPrefix(upper, "WHERE ") {
		index.predicate = flattenCondition(rest[len("WHERE "):])
	}
	return name, table, index, true
}

// balancedGroup splits a leading parenthesised group from what follows it.
func balancedGroup(text string) (inside, rest string, ok bool) {
	if !strings.HasPrefix(text, "(") {
		return "", "", false
	}
	depth := 0
	for i, char := range text {
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return text[1:i], text[i+1:], true
			}
		}
	}
	return "", "", false
}

func flattenColumns(columns string) string {
	parts := strings.Split(columns, ",")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

// flattenCondition puts a predicate in one spelling, so the catalog's
// `((a IS NULL) AND (b IS NULL))` and a statement's `a IS NULL AND b IS NULL`
// compare equal.
//
// Only an outer pair whose parentheses are each other's is dropped: stripping the
// first and last character of that catalog form leaves `a IS NULL) AND (b IS NULL`,
// which matches nothing and reads as a real finding.
func flattenCondition(condition string) string {
	flat := strings.Join(strings.Fields(strings.ToUpper(condition)), " ")
	for enclosedInOnePair(flat) {
		flat = strings.TrimSpace(flat[1 : len(flat)-1])
	}
	return flat
}

func enclosedInOnePair(condition string) bool {
	if !strings.HasPrefix(condition, "(") || !strings.HasSuffix(condition, ")") {
		return false
	}
	depth := 0
	for i, char := range condition {
		switch char {
		case '(':
			depth++
		case ')':
			depth--
		}
		// Back to zero before the end means the opening paren closed early, so the
		// two ends belong to different pairs.
		if depth == 0 && i < len(condition)-1 {
			return false
		}
	}
	return depth == 0
}

// samePredicate compares two conditions as conjunctions, because a catalog predicate
// and the statement that repeats it differ freely in grouping and in the order of
// their terms — `(a) AND (b)` is the index, `b AND a` is as good a repetition of it.
//
// A predicate carrying OR is compared whole instead: there the grouping decides what
// it means, and treating terms as a set would call two different conditions equal.
func samePredicate(wrote, index string) bool {
	if wrote == index {
		return true
	}
	if strings.Contains(wrote, " OR ") || strings.Contains(index, " OR ") {
		return false
	}
	return everyIndexTermIsWritten(conjunctionTerms(wrote), conjunctionTerms(index))
}

// everyIndexTermIsWritten pairs each term of the index's predicate with one the
// statement wrote.
//
// A term the statement built from a Go constant — `kind = '` + Kind + `'` — reaches
// here with a BLANK quoted value, because the assembler puts a space where it could
// not read an operand. Comparing that text to the catalog's `kind = 'x'::text` is a
// finding against correct code, and skipping the statement is worse: it would be the
// silent under-recognition this gate is for. So such a term is paired by the COLUMN
// it names, and everything else in the predicate is still compared exactly.
func everyIndexTermIsWritten(wrote, index []string) bool {
	taken := make([]bool, len(wrote))
	for _, want := range index {
		matched := false
		for i, got := range wrote {
			if taken[i] {
				continue
			}
			if got == want || (hasUnreadValue(got) && namesSameColumn(got, want)) {
				taken[i], matched = true, true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// hasUnreadValue reports a term comparing a column to a blank quoted value, which is
// where the statement assembler could not read a Go constant. No index predicate in
// this tree compares a column to a blank string, so the shape is unambiguous.
func hasUnreadValue(term string) bool {
	return strings.Contains(term, "''") || strings.Contains(term, "' '")
}

// namesSameColumn reports whether two predicate terms are about the same column,
// which is the most that can be checked once a term's value is unreadable.
func namesSameColumn(a, b string) bool {
	column := func(term string) string {
		return strings.TrimSpace(strings.SplitN(term, " ", 2)[0])
	}
	return column(a) == column(b)
}

func conjunctionTerms(condition string) []string {
	terms := strings.Split(condition, " AND ")
	for i, term := range terms {
		terms[i] = flattenCondition(strings.TrimSpace(term))
	}
	slices.Sort(terms)
	return terms
}

// inferableFrom answers which index an upsert on these columns would match, and
// whether the statement's own predicate lets it.
func inferableFrom(indexes []uniqueIndex, columns string) (unconditional bool, partialOnes []uniqueIndex) {
	for _, index := range indexes {
		if index.columns != columns {
			continue
		}
		if index.predicate == "" {
			unconditional = true
			continue
		}
		partialOnes = append(partialOnes, index)
	}
	return unconditional, partialOnes
}

func TestAnUpsertOntoAPartialIndexRepeatsItsPredicate(t *testing.T) {
	t.Parallel()
	indexes := uniqueIndexesByTable(t)
	var examined int
	for _, statement := range everyUpsertInTheTree(t) {
		target := insertTarget.FindStringSubmatch(statement.sql)
		conflict := conflictTarget.FindStringSubmatch(statement.sql)
		if target == nil || conflict == nil {
			continue
		}
		columns := flattenColumns(conflict[1])
		if columns == "" {
			continue // ON CONFLICT with no inference list names a constraint instead
		}
		unconditional, partialOnes := inferableFrom(indexes[target[1]], columns)
		if unconditional || len(partialOnes) == 0 {
			continue
		}
		examined++
		assertRepeatsAPredicate(t, statement, target[1], columns, conflict[2], partialOnes)
	}
	if examined == 0 {
		t.Error("no upsert in the tree infers on a partial unique index's columns — the tree " +
			"carries such upserts, so this gate has stopped finding them and would pass " +
			"whatever anyone wrote")
	}
}

func assertRepeatsAPredicate(
	t *testing.T, statement upsertStatement, table, columns, guard string, partialOnes []uniqueIndex,
) {
	t.Helper()
	wrote := flattenCondition(guard)
	for _, index := range partialOnes {
		if samePredicate(wrote, index.predicate) {
			return
		}
	}
	want := make([]string, 0, len(partialOnes))
	for _, index := range partialOnes {
		want = append(want, index.name+" on "+index.predicate)
	}
	if wrote == "" {
		t.Errorf("%s: an upsert into %s infers on (%s), where the only unique index is partial "+
			"(%s). A partial index is matched only by a statement that repeats its WHERE, so "+
			"this infers nothing and raises 42P10 on the first repeated write",
			statement.where, table, columns, strings.Join(want, "; "))
		return
	}
	t.Errorf("%s: an upsert into %s guards its conflict with %q, which matches no partial index "+
		"on (%s): %s", statement.where, table, wrote, columns, strings.Join(want, "; "))
}

// upsertStatement is one SQL statement and where a reader will find it.
type upsertStatement struct {
	where string
	sql   string
}

// everyUpsertInTheTree collects SQL from both the places it lives: Go string
// literals, and the .sql scripts that set a stack up.
//
// Migrations are left out, and only they: a migration's statements run against the
// schema AS IT WAS AT THAT VERSION, so checking them against the head catalog would
// report a mismatch for a statement that was correct when it ran — and the
// migration that creates a partial index necessarily precedes it.
func everyUpsertInTheTree(t *testing.T) []upsertStatement {
	t.Helper()
	root := moduleRoot(t)
	tree := filepath.Dir(root)
	var found []upsertStatement
	for _, dir := range []string{filepath.Join(root, "internal"), filepath.Join(root, "cmd")} {
		for _, path := range goSourceFiles(t, dir) {
			// statementsIn rather than a per-literal reader: a statement written
			// as `"INSERT … " + onConflictClause` is ONE statement to Postgres and
			// several literals to the parser, and no literal on its own holds both
			// halves this gate compares. Reading per literal passes the tree
			// silently over exactly the form somebody reaches for when a line gets
			// long — and it splits on `;`, so two statements in one literal are not
			// paired with each other's clauses.
			file, err := gatekit.ParseFile(path, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			rel := relativeToTree(t, tree, path)
			for _, sql := range statementsIn(file) {
				found = append(found, upsertStatement{where: rel, sql: sql})
			}
		}
	}
	found = append(found, sqlScriptsBelow(t, tree, root)...)
	return found
}

// sqlScriptsBelow reads each .sql file outside the migrations as one statement
// apiece. A script is not parsed into statements: an upsert's INSERT and its ON
// CONFLICT sit together, so matching the pair within the file finds them without a
// parser, and a file holding several is covered by the first mismatch it carries.
func sqlScriptsBelow(t *testing.T, tree, root string) []upsertStatement {
	t.Helper()
	migrations := filepath.Join(root, "migrations")
	var found []upsertStatement
	err := filepath.WalkDir(tree, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" || path == migrations {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".sql") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel := relativeToTree(t, tree, path)
		// Each INSERT begins a statement; the slice runs to the next one, so an ON
		// CONFLICT is read against the insert it belongs to rather than a later one.
		text := string(body)
		starts := insertTarget.FindAllStringIndex(text, -1)
		for i, start := range starts {
			end := len(text)
			if i+1 < len(starts) {
				end = starts[i+1][0]
			}
			found = append(found, upsertStatement{where: rel, sql: text[start[0]:end]})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree for .sql files: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no INSERT found in any .sql script outside the migrations — scripts/seed-dev.sql " +
			"carries several, so this walk has stopped reaching them")
	}
	return found
}

func relativeToTree(t *testing.T, tree, path string) string {
	t.Helper()
	rel, err := filepath.Rel(tree, path)
	if err != nil {
		t.Fatalf("relative path of %s: %v", path, err)
	}
	return rel
}
