// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/migrations"
)

// Reading the migrations, as SQL rather than as text.
//
// The parity tests above compare MATRICES; this half turns a directory of
// migration files into the statements a replay can run. It knows nothing about
// roles or scopes, and the tests know nothing about dollar quoting, which is
// the line the two are split on.

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
		// Asked of the EXECUTABLE SQL. A migration that names the column only in
		// prose becomes a candidate otherwise, and then has to satisfy the
		// declaration hatch below to escape a fatal — which prose cannot,
		// because there is no statement to classify.
		body := sqlWithoutComments(migration.UpSQL)
		if !mentionsRolePermissions.MatchString(body) {
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
		written := strings.Join(statements, "\n")
		objects := dedupe(append(
			objectPath.FindAllStringSubmatch(written, -1),
			objectArrayPath.FindAllStringSubmatch(written, -1)...))
		// Every objects-path literal, whatever its shape, must be accounted for.
		// A path naming a VERB (`{objects,deal,delete}`) is a legitimate write
		// this rewind cannot undo — removing the whole object would overshoot —
		// so it is reported as out of scope rather than as a name the pattern
		// failed to read. Fatalling on it refused the verb-widening backfill that
		// assertPreStateIsNotAlreadyTheAnswer calls the likelier next one.
		declared := dedupe(append(
			anyObjectPath.FindAllStringSubmatch(written, -1),
			normalizeArrayPaths(anyObjectArrayPath.FindAllStringSubmatch(written, -1))...))
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
		// Classified on the statement with its comments removed, because this
		// reads a PREFIX. A statement opening with a block comment — which
		// splitStatements keeps, since what it returns is replayed — begins with
		// `/*` however plainly the CREATE TABLE after it declares the column,
		// and the hatch then refuses the one migration it exists for.
		bare := strings.TrimSpace(sqlWithoutComments(statement))
		if !strings.Contains(strings.ToLower(bare), "permissions") {
			continue
		}
		upper := strings.ToUpper(bare)
		if !strings.HasPrefix(upper, "CREATE TABLE") && !strings.HasPrefix(upper, "ALTER TABLE") {
			return false
		}
		sawDeclaration = true
	}
	return sawDeclaration
}

// sqlWithoutComments removes -- and /* */ comments, leaving string and
// dollar-quoted literals alone.
//
// For CLASSIFICATION only — what a migration says it does, as against what it
// says about itself. Nothing replayed passes through here: the statements that
// reach the database keep their comments, because the point of replaying them is
// that they are the migration's own text.
func sqlWithoutComments(sql string) string {
	var out strings.Builder
	var quoted, escapes bool
	var tag string
	var block int

	for i := 0; i < len(sql); i++ {
		rest := sql[i:]
		switch {
		case tag != "":
			if strings.HasPrefix(rest, tag) {
				out.WriteString(tag)
				i += len(tag) - 1
				tag = ""
				continue
			}
		case block > 0:
			if strings.HasPrefix(rest, "*/") {
				block--
				i++
				continue
			}
			if strings.HasPrefix(rest, "/*") {
				block++
				i++
				continue
			}
			continue
		case quoted:
			switch {
			case escapes && sql[i] == '\\' && i+1 < len(sql):
				out.WriteString(sql[i : i+2])
				i++
				continue
			case sql[i] == '\'':
				if i+1 < len(sql) && sql[i+1] == '\'' {
					out.WriteString("''")
					i++
					continue
				}
				quoted = false
			}
		case sql[i] == '\'':
			quoted = true
			escapes = isEStringPrefix(sql, i)
		case strings.HasPrefix(rest, "/*"):
			block++
			i++
			continue
		case strings.HasPrefix(rest, "--"):
			end := strings.IndexByte(rest, '\n')
			if end < 0 {
				i = len(sql)
				continue
			}
			i += end
			out.WriteByte('\n')
			continue
		default:
			if match := dollarTag.FindString(rest); match != "" {
				tag = match
				out.WriteString(match)
				i += len(match) - 1
				continue
			}
		}
		if block == 0 {
			out.WriteByte(sql[i])
		}
	}
	return out.String()
}

// rolePermissionStatements returns the statements in one migration that write
// role.permissions, and nothing else it does.
func rolePermissionStatements(sql string) []string {
	var out []string
	for _, statement := range splitStatements(sql) {
		// Matched on the STRIPPED copy and returned RAW. splitStatements keeps
		// comments because what it returns is replayed against a real database,
		// and the pattern would otherwise find the word in one: `UPDATE role
		// SET name = 'x' /* permissions handled elsewhere */` is a write to a
		// different column, recorded here as a grant nobody made.
		if rolePermissionWrite.MatchString(sqlWithoutComments(statement)) {
			out = append(out, statement)
		}
	}
	return out
}

// isEStringPrefix reports whether the quote at i opens an E'…' literal, where a
// backslash escapes the byte after it.
//
// The E has to be a token of its own, so the byte before it must not continue
// an identifier: `THE'x'` is not an escape string, and reading it as one would
// bring back the bug this distinction exists to fix.
func isEStringPrefix(sql string, i int) bool {
	if i == 0 || (sql[i-1] != 'E' && sql[i-1] != 'e') {
		return false
	}
	if i < 2 {
		return true
	}
	prev := sql[i-2]
	return prev != '_' &&
		(prev < '0' || prev > '9') &&
		(prev < 'a' || prev > 'z') &&
		(prev < 'A' || prev > 'Z')
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
	var escapes bool // the open literal is an E'…', where a backslash escapes
	var tag string   // the active $tag$ delimiter, empty when not dollar-quoted
	var block int    // /* */ nesting depth; Postgres allows nesting

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
			case escapes && sql[i] == '\\' && i+1 < len(sql):
				// E'…\'…': the backslash escapes the next byte, and consuming it
				// here is what stops a quote-parity flip.
				//
				// ONLY in an E-string. standard_conforming_strings has been on
				// by default since 9.1, so in a plain literal a backslash is an
				// ordinary character — and treating it as an escape ate the
				// closing quote of anything ending in one (a Windows path, a
				// regex). The splitter then ran on past the terminator and
				// merged this statement with the next, which is the silent
				// mis-split this function's own header warns about.
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
			escapes = isEStringPrefix(sql, i)
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
