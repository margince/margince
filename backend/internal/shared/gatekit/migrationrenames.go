// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Reading the migrations as if the renames had always been there.
//
// A census that derives the schema from migration TEXT reads the name a thing
// was CREATED under. A later migration can rename it, and then the census is
// looking for a name the SQL never says while the code beside it knows only the
// new one — so it finds nothing and reports the clean tree it never read. That
// is the failure mode a census must not have, because under-recognition is
// silent.
//
// The substitutions are derived from the `ALTER ... RENAME` statements
// themselves, never listed here: a gate that hard-codes part of its subject has
// become a second copy of it, and a list would go stale at the next rename.
//
// It lives here rather than beside any one census because three of them read
// the same SQL — the gates package, the search join census and the owner-private
// census in platform/auth — and two spellings of one derivation drift.

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

var (
	renamesOnce  sync.Mutex
	renamesCache = map[string]map[string]string{}
)

// CurrentNames answers, for the core migrations under dir, where each renamed
// identifier ended up. Cached per directory: every caller passes a path of its
// own depth, and walking 460 files once per census is the only cost worth
// avoiding here.
func CurrentNames(dir string) map[string]string {
	renamesOnce.Lock()
	defer renamesOnce.Unlock()
	if cached, ok := renamesCache[dir]; ok {
		return cached
	}
	built := buildRenames(dir)
	renamesCache[dir] = built
	return built
}

// buildRenames walks the core migrations in apply order and collects every
// rename, following a chain so a name moved twice lands on where it is now.
func buildRenames(dir string) map[string]string {
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		return nil
	}
	sort.Strings(files)

	renames := map[string]string{}
	for _, file := range files {
		body, err := os.ReadFile(file) // #nosec G304 -- a *.up.sql name from the migrations tree, test-support only
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

// WithCurrentNames rewrites migration SQL so every renamed identifier reads
// under the name the database has today. A census scanning the result sees the
// schema it is actually judging rather than the one the baseline first built.
func WithCurrentNames(dir, sql string) string {
	renames := CurrentNames(dir)
	if len(renames) == 0 {
		return sql
	}
	// Longest first: contact_channel_identity must move before contact, or the
	// shorter match leaves a mangled tail.
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
