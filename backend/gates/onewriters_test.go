// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Each fact here has one writer, and a second one fails.
//
// A single writer keeps the audit row, the event, the column list or the one
// guarding word with the write. A second writer agrees with the first until
// one of them is edited. Each row names the fact, the place that writes it,
// and the count of statements that place holds. The count must match, because
// zero is how a census fails silently when the writer moves.
//
// Every row reads the same corpus through gatekit.SQLStatementsOf, one
// declaration at a time: escapes decoded, `+` chains joined, and SQL inside a
// non-literal operand still read. What it cannot see is a statement whose verb
// and table both arrive through a variable at run time.

import (
	"fmt"
	"go/ast"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// oneWriter is one row: a fact, how a statement writes it, and the one place
// that may.
type oneWriter struct {
	fact string
	// writes counts the writes of the fact in one statement.
	writes func(statement string) int
	// owner is a file, or file:function, relative to the backend module.
	owner       string
	ownerWrites int
	// tests also judges _test.go files. plantedIn are the gate files that
	// spell the statement as a planted case, which a tests row skips.
	tests     bool
	plantedIn []string
	// exempt are files that may also write the fact, each with its reason.
	exempt *gatekit.Waivers[string]
	why    string
	// planted is one write of the fact, for the planted cases.
	planted string
}

// oneWriterRoots are the Go trees every row reads: the backend module and the
// trees beside it that ship the same way.
var oneWriterRoots = []string{".", "../extensions", "../fixtures"}

var oneWriters = []oneWriter{
	{
		fact: "project_health_assessment", writes: insertsInto("project_health_assessment"),
		owner: "internal/modules/projects/health.go:insertHealthRow", ownerWrites: 1,
		exempt:  gatekit.Waive(map[string]string{}),
		why:     "append through insertHealthRow, which writes the audit row and the event with it",
		planted: "INSERT INTO project_health_assessment (id) VALUES ($1)",
	},
	{
		fact: "voice_profile_version", writes: insertsInto("voice_profile_version"),
		owner: "internal/modules/ai/voice_version_write.go:insertVoiceVersion", ownerWrites: 1,
		exempt: gatekit.Waive(map[string]string{}),
		why: "one writer owns the column list, so a column added to the table is a field nobody fills " +
			"rather than a default nobody notices",
		planted: "INSERT INTO voice_profile_version (voice_profile_id) VALUES ($1)",
	},
	{
		fact: "voice_profile_delta", writes: insertsInto("voice_profile_delta"),
		owner: "internal/modules/ai/voice_version_write.go:insertVoiceDelta", ownerWrites: 1,
		exempt: gatekit.Waive(map[string]string{}),
		why: "one writer owns the column list, so a column added to the table is a field nobody fills " +
			"rather than a default nobody notices",
		planted: "INSERT INTO voice_profile_delta (voice_profile_id) VALUES ($1)",
	},
	{
		fact: "a user's time zone", writes: func(statement string) int { return boolCount(assignsTimezone(statement)) },
		owner: "internal/modules/identity/usertimezone.go", ownerWrites: 1,
		// A row being created has no stored zone a browser guess could replace.
		exempt: gatekit.Waive(map[string]string{
			"internal/modules/identity/installation.go": "creates the first admin's row with the zone the installation was set up in",
		}),
		why: "writeTimezoneTx keeps a chosen zone from being replaced by a browser guess and sets " +
			"timezone_chosen with the zone",
		planted: "UPDATE app_user SET timezone = $2 WHERE id = $1",
	},
	{
		fact: "user_record_view", writes: matchesOf(writesViewBaseline),
		owner: viewBaselineOwner, ownerWrites: 2, tests: true,
		plantedIn: []string{"gates/userrecordviewwriter_test.go", "gates/onewriters_test.go"},
		exempt:    gatekit.Waive(map[string]string{}),
		why: "the mark's correctness is GREATEST(stored, EXCLUDED), and a statement that loses it rewinds " +
			"a baseline on a late ack. Call company360.RecordVisit after your own visibility gate",
		planted: "UPDATE user_record_view SET last_viewed_at = $1",
	},
	{
		fact: "contact_confirm_submission", writes: insertsInto("contact_confirm_submission"),
		owner: "internal/modules/consent/confirmsubmit.go:stageSubmission", ownerWrites: 1,
		exempt: gatekit.Waive(map[string]string{}),
		why: "every proposal a subject sends must reach the rights case stageOneProposal opens with it, " +
			"and one writer is what makes that provable",
		planted: "INSERT INTO contact_confirm_submission (contact_id) VALUES ($1)",
	},
	{
		fact: "communication_override", writes: matchesOf(overrideInsert),
		owner: standingExceptionCarry, ownerWrites: 1, exempt: gatekit.Waive(map[string]string{}),
		why:     "no door records a new standing exception; only the merge carry copies rows already on file",
		planted: "INSERT INTO communication_override (id) VALUES ($1)",
	},
}

// insertsInto counts the INSERTs into one table, in any case or spacing.
func insertsInto(table string) func(string) int {
	return matchesOf(regexp.MustCompile(`(?is)INSERT\s+INTO\s+` + regexp.QuoteMeta(table) + `\b`))
}

// matchesOf counts every match, so two statements in one literal are two writers.
func matchesOf(pattern *regexp.Regexp) func(string) int {
	return func(statement string) int { return len(pattern.FindAllStringIndex(statement, -1)) }
}

func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestEachFactHasOneWriter is the census the writers' doc comments name.
func TestEachFactHasOneWriter(t *testing.T) {
	t.Parallel()
	files, read := oneWriterCorpus(t)
	// A walk that reads nothing passes like a clean tree.
	if read < 500 {
		t.Fatalf("the census read only %d Go files, so it covered almost nothing", read)
	}
	for _, row := range oneWriters {
		t.Run(row.fact, func(t *testing.T) {
			t.Parallel()
			inOwner, outside := 0, []string{}
			for _, parsed := range files {
				if !row.reads(parsed.Path) {
					continue
				}
				for _, site := range row.sites(parsed) {
					switch {
					case site.subject == row.owner || fileOf(site.subject) == row.owner:
						inOwner += site.writes
					case !row.exempt.Waived(t, parsed.Path):
						outside = append(outside, fmt.Sprintf("%s (%d)", site.subject, site.writes))
					}
				}
			}
			for _, site := range outside {
				t.Errorf("%s writes %s outside %s: %s", site, row.fact, row.owner, row.why)
			}
			if inOwner != row.ownerWrites {
				t.Errorf("%s holds %d write(s) of %s, want exactly %d: the writer moved or stopped being "+
					"spelled whole, and \"no second writer\" reads the same over that tree", row.owner, inOwner,
					row.fact, row.ownerWrites)
			}
			row.exempt.AssertAllMatched(t)
		})
	}
}

// oneWriterCorpus parses every Go source under the roots, tests included, and
// counts the files it read.
func oneWriterCorpus(t *testing.T) ([]gatekit.ParsedFile, int) {
	t.Helper()
	var files []gatekit.ParsedFile
	for _, root := range oneWriterRoots {
		err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_gen.go") {
				return walkErr
			}
			file, err := gatekit.ParseFile(path, 0)
			if err != nil {
				return err
			}
			files = append(files, gatekit.ParsedFile{Path: filepath.ToSlash(path), File: file})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	return files, len(files)
}

// reads reports whether the row judges a file: tests only for a tests row, and
// never the gate files that plant its statement.
func (w oneWriter) reads(path string) bool {
	if strings.HasSuffix(path, "_test.go") && !w.tests {
		return false
	}
	return !slices.Contains(w.plantedIn, path)
}

// writeSite is one declaration that writes the fact: file:function, or the file
// for a package-level value.
type writeSite struct {
	subject string
	writes  int
}

// sites counts the fact's writes per declaration. The whole file is asked
// first, which is the shared memo, and most files write nothing.
func (w oneWriter) sites(parsed gatekit.ParsedFile) []writeSite {
	if !slices.ContainsFunc(gatekit.SQLStatementsOf(parsed.File), func(s string) bool { return w.writes(s) > 0 }) {
		return nil
	}
	var sites []writeSite
	for _, decl := range parsed.File.Decls {
		n := 0
		for _, statement := range gatekit.SQLStatementsOf(decl) {
			n += w.writes(statement)
		}
		if n == 0 {
			continue
		}
		subject := parsed.Path
		if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
			subject += ":" + fn.Name.Name
		}
		sites = append(sites, writeSite{subject: subject, writes: n})
	}
	return sites
}

// TestEachWriterCensusSeesASecondWriter plants, per row, a write spelled whole,
// one split across a `+` chain, and one held in a package constant. Each is a
// second writer, and a read of the fact is none.
func TestEachWriterCensusSeesASecondWriter(t *testing.T) {
	t.Parallel()
	for _, row := range oneWriters {
		t.Run(row.fact, func(t *testing.T) {
			t.Parallel()
			planted := row.planted
			half := len(planted) / 2
			fixture := parseGateFixture(t, "package p\n"+
				"func whole() { q(`"+planted+"`) }\n"+
				"func split() { q(\""+planted[:half]+"\" + \""+planted[half:]+"\") }\n"+
				"const held = `"+planted+"`\n"+
				"func reads() { q(`SELECT 1 FROM app_user, project_health_assessment, voice_profile_version`) }\n")
			got := row.sites(gatekit.ParsedFile{Path: "planted.go", File: fixture})
			want := []writeSite{{"planted.go:whole", 1}, {"planted.go:split", 1}, {"planted.go", 1}}
			if !slices.Equal(got, want) {
				t.Errorf("the %s census found %v, want %v", row.fact, got, want)
			}
		})
	}
}
