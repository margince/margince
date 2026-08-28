// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// user_record_view carries one fact per (user, record): the moment that human
// last said "I have seen this". Every 360 surface reports its since-last-visit
// counts against it, so the mark is the baseline every "new since you were
// here" answer is measured from.
//
// THE CORRECTNESS IS IN ONE WORD. `GREATEST(stored, EXCLUDED)` is what stops a
// slow tab's late-arriving acknowledgement rewinding a newer one: two tabs open
// on the same record converge on the later visit instead of racing the baseline
// backwards. A copy that lost it — by being written from memory, by being
// simplified, by a merge resolving the wrong way — would silently hand a reader
// back an unread marker they had already consumed, on a surface where "3 new"
// and "0 new" are both plausible and neither looks like a bug.
//
// So the statement lives once, and this is the test that holds it. Two writers
// of one write shape signal nothing while they agree, which is the whole
// window: they agree right up until the moment one of them is edited.
//
// What the callers keep is the part that legitimately differs: their own
// visibility gate. org360 asks `EnsureVisible`; person360 asks
// `EnsureVisibleLive`, because Art. 17 anonymizes a person in place while
// leaving owner_id alone and the plain probe would still admit them. That is a
// ruling per record type. The upsert is not.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// viewBaselineOwner is the file that may write the table. Keyed to the FILE and
// not the package: a second file in org360 writing its own upsert is the same
// defect wearing the right import path.
const viewBaselineOwner = "internal/compose/org360/viewbaseline.go"

// writesViewBaseline matches a statement that INSERTs or UPDATEs the table.
//
// Both verbs, because the defect is not "a second INSERT" — it is a second
// answer to "how does this mark move". A bare `UPDATE user_record_view SET
// last_viewed_at = $1` carries no GREATEST at all and is the worst possible
// second writer: it rewinds unconditionally.
//
// The name may be QUALIFIED and quoted in more ways than one, and a pattern
// anchored on the bare word passes every other spelling exactly like a clean
// tree. Postgres accepts all of these, and each is planted below:
//
//	public.user_record_view          one qualifier
//	tenant1.public.user_record_view  two — so the qualifier repeats
//	public . user_record_view        whitespace around the separator
//	"public"."user_record_view"      quoted, in any combination
//	UPDATE ONLY user_record_view     the inheritance-scoped form
//
// The NAME's end is a delimiter, not `\b`. `$`, `.` and `"` are all legal
// around a Postgres identifier and none is a word character. The delimiter
// excludes every character that can CONTINUE or QUALIFY a name, so a trailing
// `.` or `"` is refused whether the target was quoted or not —
// `user_record_view$archive` and `user_record_view.extra` are other relations.
//
// And the quoted branch is case-SENSITIVE while the rest is not, because that
// is Postgres: an unquoted `USER_RECORD_VIEW` folds to this table, a quoted
// `"USER_RECORD_VIEW"` is a different one. Go's regexp has no lookahead, so the
// distinction is two branches rather than a negation.
//
// None of it costs the near-neighbour: `user_record_view_archive` continues
// with `_`, which the unquoted branch refuses. Its bare, qualified and ONLY
// forms are all planted as must-MISS.
var writesViewBaseline = regexp.MustCompile(
	`(?is)(INSERT\s+INTO|UPDATE(?:\s+ONLY)?)\s+` +
		`(?:"?[\w$]+"?\s*\.\s*)*(?:(?-i:"user_record_view")|user_record_view)(?:[^\w$".]|$)`)

// TestUserRecordViewHasOneWriter is the census `org360.RecordVisit`'s doc
// comment names.
func TestUserRecordViewHasOneWriter(t *testing.T) {
	t.Parallel()
	var findings []string
	judged := 0
	for _, root := range []string{".", "../extensions", "../fixtures"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "node_modules" || name == "testdata" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			rel := filepath.ToSlash(path)
			if rel == viewBaselineOwner {
				return nil
			}
			// This file spells the statement out in its own probes below;
			// judging them would report the gate's own evidence as a finding.
			if filepath.Base(path) == "userrecordviewwriter_test.go" {
				return nil
			}
			source, readErr := os.ReadFile(path) // #nosec G304 -- a *.go path from walking the trusted source tree
			if readErr != nil {
				return readErr
			}
			judged++
			for _, line := range gatekit.SQLStatementsIn(t, path, string(source)) {
				if writesViewBaseline.MatchString(line) {
					findings = append(findings, rel+": "+gatekit.FirstLineOf(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	// A walk that reads nothing passes exactly like a clean tree.
	if judged < 500 {
		t.Fatalf("the census read only %d Go files, so it covered almost nothing", judged)
	}
	if len(findings) > 0 {
		t.Errorf("%d statement(s) write user_record_view outside %s.\n\n"+
			"The mark's correctness is one word — GREATEST(stored, EXCLUDED) — and a second "+
			"statement that loses it rewinds a baseline on a late-arriving ack, handing a reader "+
			"back an unread marker they had already consumed. Call org360.RecordVisit inside your "+
			"own transaction, after your own visibility gate:\n\n\t%s",
			len(findings), viewBaselineOwner, strings.Join(findings, "\n\t"))
	}
}

// TestTheViewBaselineCensusSeesWhatItClaimsTo plants what the census must
// catch, and what it must not.
//
// A gate asserting a shape is ABSENT passes identically over a clean tree and
// over a detector that has stopped detecting, so the detector needs its own
// evidence.
func TestTheViewBaselineCensusSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	caught := []string{
		"INSERT INTO user_record_view (user_id, entity_type, entity_id, last_viewed_at)",
		// The worst second writer: no GREATEST at all, so it rewinds.
		"UPDATE user_record_view SET last_viewed_at = $1 WHERE user_id = $2",
		// Case and whitespace are not the subject.
		"insert   into\n\t\tuser_record_view (user_id)",
		// Schema-qualified, and quoted. A pattern anchored on the bare word
		// waves both through and reports the tree clean.
		"INSERT INTO public.user_record_view (user_id) VALUES ($1)",
		`UPDATE "user_record_view" SET last_viewed_at = $1`,
		`INSERT INTO "public"."user_record_view" (user_id) VALUES ($1)`,
		// A three-part name, and whitespace around the separator. Both are
		// valid Postgres, so both are writes the census must see.
		"INSERT INTO tenant1.public.user_record_view (user_id) VALUES ($1)",
		`UPDATE "tenant1"."public"."user_record_view" SET last_viewed_at = $1`,
		"INSERT INTO public . user_record_view (user_id) VALUES ($1)",
		// UPDATE ONLY: inheritance-scoped, and it rewinds just as hard.
		"UPDATE ONLY user_record_view SET last_viewed_at = $1",
		"UPDATE ONLY public.user_record_view SET last_viewed_at = $1",
		// The statement may simply END after the name.
		"UPDATE user_record_view",
	}
	for _, statement := range caught {
		if !writesViewBaseline.MatchString(statement) {
			t.Errorf("the census does not see a write it must:\n\t%s", statement)
		}
	}
	missed := []string{
		// A READ is not a write. Every 360 reads this table to compute its
		// since-last-visit counts, and reporting those would bury the finding.
		"SELECT last_viewed_at FROM user_record_view WHERE user_id = $1",
		// A different table whose name merely contains this one's would be a
		// false positive; the word boundary is what stops it.
		"INSERT INTO user_record_view_archive (user_id) VALUES ($1)",
		// And no amount of qualifying may widen the near-neighbour: `_` is a
		// word character, so the boundary after the name holds.
		"INSERT INTO public.user_record_view_archive (user_id) VALUES ($1)",
		"INSERT INTO tenant1.public.user_record_view_archive (user_id) VALUES ($1)",
		"UPDATE ONLY user_record_view_archive SET last_viewed_at = $1",
		// `$` and `.` are legal in a Postgres identifier and are not word
		// characters, so a `\b` alone reported both of these as this table.
		"INSERT INTO user_record_view$archive (user_id) VALUES ($1)",
		`INSERT INTO "user_record_view.extra" (user_id) VALUES ($1)`,
		// A QUOTED identifier is case-sensitive in Postgres, so this names a
		// different table than the one the gate protects.
		`INSERT INTO "USER_RECORD_VIEW" (user_id) VALUES ($1)`,
		// A trailing qualifier names something else again: here the target is
		// the SCHEMA, not the table.
		"INSERT INTO user_record_view.extra (user_id) VALUES ($1)",
		`INSERT INTO "user_record_view".extra (user_id) VALUES ($1)`,
		`INSERT INTO "user_record_view""archive" (user_id) VALUES ($1)`,
	}
	for _, statement := range missed {
		if writesViewBaseline.MatchString(statement) {
			t.Errorf("the census reports something that is not a second writer:\n\t%s", statement)
		}
	}
}
