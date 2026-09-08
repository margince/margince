// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Reading the migrations as if the renames had always been there.
//
// A gate that derives the schema from migration TEXT reads the name a thing was
// CREATED under. A later migration can rename it, and then the gate is looking
// for a name the SQL never says while the code beside it knows only the new
// one — so the census finds nothing and reports the clean tree it never read.
// That is the failure mode a census must not have.
//
// The substitutions are derived from the `ALTER ... RENAME` statements
// themselves, never listed here: a gate that hard-codes part of its subject has
// become a second copy of it, and this one would go stale at the next rename.
//
// This lives in a NON-test file so it is compiled under every build tag. A
// helper behind `//go:build !integration` is invisible to `make lint`, which
// sets that tag.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// renameStatements are the four verbs that move a name in this tree's SQL. Each
// captures the old name then the new one.
var renameStatements = []*regexp.Regexp{
	regexp.MustCompile(`(?is)ALTER\s+TABLE\s+(?:ONLY\s+)?(\w+)\s+RENAME\s+TO\s+(\w+)`),
	regexp.MustCompile(`(?is)ALTER\s+TABLE\s+(?:ONLY\s+)?\w+\s+RENAME\s+(?:COLUMN\s+)?(\w+)\s+TO\s+(\w+)`),
	regexp.MustCompile(`(?is)ALTER\s+TABLE\s+(?:ONLY\s+)?\w+\s+RENAME\s+CONSTRAINT\s+(\w+)\s+TO\s+(\w+)`),
	regexp.MustCompile(`(?is)ALTER\s+(?:INDEX|FUNCTION|TRIGGER)\s+(\w+)(?:\(\))?(?:\s+ON\s+\w+)?\s+RENAME\s+TO\s+(\w+)`),
}

var currentNames = sync.OnceValue(buildRenames)

// buildRenames walks the core migrations in apply order and collects every
// rename, following a chain so a name moved twice lands on where it is now.
func buildRenames() map[string]string {
	files, err := filepath.Glob(filepath.Join(repoRoot, "backend", "migrations", "core", "*.up.sql"))
	if err != nil || len(files) == 0 {
		return nil
	}
	sort.Strings(files)

	renames := map[string]string{}
	for _, file := range files {
		body, err := os.ReadFile(file) // #nosec G304 -- a *.up.sql name from the migrations tree, test-only
		if err != nil {
			continue
		}
		for _, verb := range renameStatements {
			for _, m := range verb.FindAllStringSubmatch(string(body), -1) {
				old, now := m[1], m[2]
				if old == now {
					continue
				}
				// A name that was itself the target of an earlier rename keeps
				// one entry, pointing at where it ended up.
				for from, to := range renames {
					if to == old {
						renames[from] = now
					}
				}
				renames[old] = now
			}
		}
	}
	return renames
}

// asCreated answers what a thing now called name was called in the migration
// that created it, so a text scan can find the statement that made it.
func asCreated(name string) string {
	for old, now := range currentNames() {
		if now == name {
			return old
		}
	}
	return name
}

// withCurrentNames rewrites migration SQL so every renamed identifier reads
// under the name the database has today. A gate scanning the result sees the
// schema it is actually judging rather than the one the baseline first built.
func withCurrentNames(sql string) string {
	renames := currentNames()
	if len(renames) == 0 {
		return sql
	}
	// Longest first: company_domain_disposition must move before
	// company_domain, or the shorter match leaves a mangled tail.
	olds := make([]string, 0, len(renames))
	for old := range renames {
		olds = append(olds, old)
	}
	sort.Slice(olds, func(i, j int) bool { return len(olds[i]) > len(olds[j]) })

	for _, old := range olds {
		if !strings.Contains(sql, old) {
			continue
		}
		sql = regexp.MustCompile(`\b`+regexp.QuoteMeta(old)+`\b`).ReplaceAllString(sql, renames[old])
	}
	return sql
}
