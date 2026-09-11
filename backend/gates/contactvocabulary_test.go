// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

//go:build !integration

package gates

// The record is called a contact, and this is what stops the other word coming
// back.
//
// The screen half was done first and stopped at the last layer a reader can
// see: the browser said `#/contacts/:id` and the request it fired said
// `GET /people/{id}`, the table was `person`, the public events were
// `person.created`, and the search DSL had a third spelling again in `persons`.
// Nothing failed, because nothing was looking — which is why the fix is a test
// and not a sweep. frontend/src/i18n/record-noun.test.ts holds the copy in both
// directions and had no counterpart below it; this is that counterpart.
//
// WHAT THIS DOES NOT COVER, said rather than left to be assumed. The word
// `contact` itself, which this tree already used for four other things before
// the rename — a `graph_contact_edge`, a `company_fact` whose field is
// `contact_email`, a partner's `last_contact_at`, a lead that has been
// `contacted`. Telling a NEW second spelling of the record apart from those is
// judgement per occurrence, so this test does not pretend to: it refuses the
// retired word and says nothing about the new one.
//
// The corpus comes from the index, so a new file is in scope automatically and
// an untracked scratch file is not.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// theRetiredWord matches the record's former names. `person` and `people`
// only — `persons`, the search DSL's third spelling, is `person` with a
// suffix and needs no arm of its own.
var theRetiredWord = regexp.MustCompile(`(?i)person|people`)

// notThisRecord is the words that merely spell those letters: a mask, an
// adjective, a payroll noun, the German for persons, a forgery, a job title,
// and the one that means physically present. Removed before the match rather
// than waived per site, because they are a property of the language and not of
// any one file.
//
// Each is a STEM and not a next-character rule, which is the case this test was
// written without. A lowercase run has no boundary in it: `personname.go` and
// `personaccess.tsx` are the record, and a predicate that read "person followed
// by a or n" protected the record's own file names from it and reported a clean
// tree over a planted one.
var notThisRecord = regexp.MustCompile(
	`(?i)personal\w*|personnel\w*|personas?(?:[^a-z]|$)|personen\w*` +
		`|impersonat\w*|salespe(?:rson|ople)\w*` +
		// Physically present. The separator decides how far this reaches:
		// `in_person` and `InPerson` are one name, so nothing after them
		// matters, while a prose "in person" ends where the word ends — "it
		// lands in person_social" names a table and "we met in person" does not.
		`|[Ii][Nn][_-][Pp]erson\w*|[Ii]n[Pp]erson\w*` +
		`|(?:^|[^A-Za-z])[Ii][Nn] [Pp]erson(?:[^A-Za-z0-9_]|$)`)

// humanSense is the sentences where the catalog says the word about a HUMAN
// BEING rather than about this record — a colleague, a sender, an attendee,
// somebody with a seat here.
//
// DERIVED from the catalogs rather than listed, because they already have an
// owner: frontend/src/i18n/record-noun.test.ts names every such key, per
// locale, with the reason the word stays, and holds each from both sides. A
// second copy here would be a second answer to one question, and the one that
// went stale would be this one.
//
// Only a value of ten characters or more, and only as far as its first
// placeholder: `People` and `Person` are labels short enough to appear as
// ordinary words anywhere, and stripping them would take the gate's subject
// with them.
var humanSense = sync.OnceValue(humanSenseSentences)

func humanSenseSentences() []string {
	entry := regexp.MustCompile(`(?m)^\s*"(?:[^"\\]|\\.)+":\s*"((?:[^"\\]|\\.)*)",\s*$`)
	seen := map[string]bool{}
	for _, locale := range []string{"en", "de", "vi"} {
		body, err := os.ReadFile(filepath.Join( // #nosec G304 -- a fixed catalog name
			repoRoot, "frontend", "src", "i18n", locale+".ts"))
		if err != nil {
			continue
		}
		for _, m := range entry.FindAllStringSubmatch(string(body), -1) {
			value, _, _ := strings.Cut(m[1], "{")
			value = strings.TrimSpace(value)
			if len(value) >= 10 && namesTheRetiredWord(value) {
				seen[value] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	// Longest first, so a sentence that contains a shorter one is taken whole.
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// frozenCriteria is the judge questions a recorded verdict has already
// answered, which this rename is not free to reword.
//
// A verdict under e2e/llm/testdata/judge is filed under sha256(criterion +
// answer), so a criterion reworded by one word MISSES its recording and the
// checker refuses to run rather than replay a verdict about a question nobody
// asked — the direction e2e/llm/judge.py deliberately fails in. Re-asking one
// costs a real model call, which the Makefile keeps opt-in because it costs
// money. The sibling rename settled the same point by touching no criterion at
// all; this says why in a form that fails.
//
// DERIVED from the verdict corpus and not listed, so re-recording a verdict
// narrows this by itself. The recorded criterion is the WHOLE of what is
// frozen: a scenario's own comments are not, and stay in scope. A corpus this
// cannot read yields nothing and makes the census louder, never quieter, which
// is the only direction an exemption may fail in.
var frozenCriteria = sync.OnceValue(recordedCriteria)

func recordedCriteria() []string {
	dir := filepath.Join(repoRoot, "e2e", "llm", "testdata", "judge")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(dir, entry.Name())) // #nosec G304 -- a corpus this test owns
		if err != nil {
			continue
		}
		var recorded struct {
			Criterion string `json:"criterion"`
		}
		if json.Unmarshal(body, &recorded) != nil {
			continue
		}
		if namesTheRetiredWord(recorded.Criterion) {
			seen[recorded.Criterion] = true
		}
	}
	out := make([]string, 0, len(seen))
	for criterion := range seen {
		out = append(out, criterion)
	}
	sort.Strings(out)
	return out
}

// retired names a path and says why the word is right there. The reason is the
// point: a waiver nobody had to justify is how a second spelling survives
// review for a year.
var retired = gatekit.Waive(map[string]string{
	"backend/migrations/core": "shipped migrations are never edited — the SQL that built the " +
		"old names is the record of how the schema got here",
	"backend/migrations/custom": "the same, one namespace over",
	"backend/migrations/testdata/rbac_baseline_era_defaults.json": "pinned byte for byte to the " +
		"baseline commit by rbacbaselineerafixture_test.go; it IS the matrix the server seeded then",
	"backend/migrations/enginebackfill_integration_test.go": "it REPLAYS a shipped migration, " +
		"and names the columns back for the length of the replay so the real file runs rather " +
		"than a retyped copy of it",
	"CHANGELOG.md": "entries say what they said when they were written",
	"sbom-schemas": "a vendored SPDX schema, not ours to rename",
	"e2e/llm/testdata": "recorded model output — what a model actually said on a run, which " +
		"editing would falsify",

	"frontend/src/i18n/en.ts": "the catalog's human-sense values, held key by key and from both " +
		"sides by frontend/src/i18n/record-noun.test.ts, which is where the reason each one keeps " +
		"the word is written down",
	"frontend/src/i18n/de.ts":               "the same catalog, one locale over",
	"frontend/src/i18n/vi.ts":               "the same catalog, one locale over",
	"frontend/src/i18n/record-noun.test.ts": "it names both words in order to hold one of them",

	// Settings → People is the SEATS group, beside Company and Sales. It heads
	// the pages about colleagues with a licence, and it is the one surface in
	// this tree where the plural still means the humans who work here.
	"frontend/src/screens/settingscatalog.ts":              "the seats settings group",
	"frontend/src/app/shell.stories.tsx":                   "the seats settings group",
	"frontend/src/screens/settings-chrome.test.tsx":        "the seats settings group",
	"frontend/src/screens/license.stories.tsx":             "a page under the seats settings group",
	"frontend/src/screens/licenseholder.stories.tsx":       "a page under the seats settings group",
	"frontend/src/screens/users-access.stories.tsx":        "a page under the seats settings group",
	"frontend/src/screens/users-admin.stories.tsx":         "a page under the seats settings group",
	"frontend/src/screens/users-invite-form.stories.tsx":   "a page under the seats settings group",
	"frontend/src/screens/users-password-link.stories.tsx": "a page under the seats settings group",
	"frontend/src/screens/share.tsx": "the share target kind: a colleague with a seat, or a team " +
		"of them — never this record",

	"scripts/fe-file-length-waivers.txt": "its note explains which two words got wider, which it " +
		"cannot do without saying them",

	"backend/internal/compose/auditlegacytype.go": "the word audit rows written before the " +
		"rename still carry. `trg_audit_no_mutate` refuses an UPDATE on audit_log, so those rows " +
		"keep it forever and the two reads that filter the trail by record type have to match it " +
		"— the file exists to say so once instead of twice",

	"docs/reference/record-vocabulary.md": "it states the rule, which it cannot do without " +
		"naming the word the rule retires",

	"backend/gates/contactvocabulary_test.go": "this file names the word in order to refuse it",
})

// beyondTheExemptions is the part of a line this census still judges: what is
// left once the sentences that have another owner are taken out of it.
//
// Taken out of the line rather than skipping the line, because both exemptions
// are SENTENCES and a line carries more than one thing. A scenario comment
// quoting a frozen criterion and then naming `person_id` is still naming the
// column, and a skip-the-line rule would read that as clean.
func beyondTheExemptions(line string) string {
	for _, sentence := range humanSense() {
		line = strings.ReplaceAll(line, sentence, "")
	}
	for _, criterion := range frozenCriteria() {
		line = strings.ReplaceAll(line, criterion, "")
	}
	return line
}

func TestTheRecordIsCalledAContact(t *testing.T) {
	t.Parallel()
	// A waiver that stopped matching reads as ratification of code that is
	// gone, so the full sweep below is the one place that can say so.
	defer retired.AssertAllMatched(t)

	read := 0
	for _, f := range trackedFiles(t) {
		if f.symlink || !readableAsText(f.path) {
			continue
		}
		if retirementCovers(t, f.path) {
			continue
		}
		// The PATH says the word as loudly as the contents do, and a census
		// that read only the contents let personname.go, personaccess.tsx and
		// personautoenrich.go through — three files whose bodies had been
		// renamed and whose names had not.
		if pathNamesTheRetiredWord(f.path) {
			t.Errorf("%s is NAMED for the record's retired name. It is called a contact.", f.path)
		}
		body, err := os.ReadFile(filepath.Join(repoRoot, f.path))
		if err != nil {
			t.Fatalf("reading %s: %v", f.path, err)
		}
		if binary(body) {
			continue
		}
		read++
		for i, line := range strings.Split(string(body), "\n") {
			if !namesTheRetiredWord(beyondTheExemptions(line)) {
				continue
			}
			t.Errorf("%s:%d calls the record by its retired name:\n\t%s\n"+
				"It is called a contact. If this one is something else — a personal mail "+
				"verdict, an in-person exchange, the seats settings group — add the path to "+
				"retired, with the reason.",
				f.path, i+1, strings.TrimSpace(line))
		}
	}

	// Under-recognition is the one way this can fail without failing: read a
	// smaller tree, report PASS, and no assertion notices.
	if read < 1000 {
		t.Fatalf("the census read %d files, too few to be this tree — a scan that comes up "+
			"short reports the clean result it never looked for", read)
	}
	if len(humanSense()) < 50 {
		t.Fatalf("the census derived %d human-sense sentences from the catalogs, too few to be "+
			"the list record-noun.test.ts holds — a derivation that comes up short makes this "+
			"gate louder, not quieter, and the next author turns it off", len(humanSense()))
	}
}

// TestTheCensusSeesAConcatenatedName plants the case the stem guards exist for.
// Under-recognition is silent: a guard widened by one character would leave the
// census reporting a clean tree, and no assertion above would notice.
func TestTheCensusSeesAConcatenatedName(t *testing.T) {
	t.Parallel()

	for _, named := range []string{
		"backend/internal/modules/people/doc.go",
		"frontend/src/screens/personname.tsx",
		"frontend/src/screens/personaccess.tsx",
		"backend/internal/compose/personautoenrich.go",
		"backend/internal/shared/ports/persondata/port.go",
	} {
		if !pathNamesTheRetiredWord(named) {
			t.Errorf("%s is named for the retired word and the path check does not see it", named)
		}
	}

	for _, innocent := range []string{
		"backend/internal/modules/capture/domainpersonal.go",
		"backend/internal/compose/capturepersonalpurge_integration_test.go",
		"backend/internal/modules/ai/personality.go",
		"frontend/src/screens/personalpurgewindow.test.ts",
	} {
		if pathNamesTheRetiredWord(innocent) {
			t.Errorf("%s is ordinary English and the path check reads it as the retired word", innocent)
		}
	}

	for _, line := range []string{
		`func (s *Store) EnsurePersonByEmail(ctx context.Context) error {`,
		`	var in persondraft.Input`,
		`  it lands in person_social and no column of the human moves`,
		`const ROUTABLE = new Set(["deal", "person"]);`,
		`ALTER TABLE person RENAME TO contact;`,
		`	PersonID ids.PersonID ` + "`json:\"person_id\"`",
		`// the structs in person360 are hand-written`,
	} {
		if !namesTheRetiredWord(line) {
			t.Errorf("the census reads past the retired word in:\n\t%s", line)
		}
	}

	for _, innocent := range []string{
		`	// DeletesAt Present only for a ` + "`personal`" + ` verdict`,
		`	ConsentQualifyingEventKindInPerson ConsentQualifyingEventKind = "in_person"`,
		`		"requested_quote_or_meeting", "in_person_permission":`,
		`	voiceKeyPersonalityMD = "personality_md"`,
		`	confidentialityPersonnel = "personnel"`,
		`// a salesperson reading the queue`,
		`	if principal.Impersonating() {`,
		`  "co.notice.personenbezogene": "Personenbezogene Daten",`,
		`			t.Fatalf("a card handed over in person carries no message")`,
	} {
		if namesTheRetiredWord(innocent) {
			t.Errorf("the census reads ordinary English as the retired word in:\n\t%s", innocent)
		}
	}
}

// pathNamesTheRetiredWord reports whether a path segment is named for the
// record's retired name. It shares the line predicate, because both face the
// same problem: a Go file name is one lowercase run with no boundary in it.
func pathNamesTheRetiredWord(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if namesTheRetiredWord(segment) {
			return true
		}
	}
	return false
}

// namesTheRetiredWord reports whether a line calls this record by a retired
// name, once the words that merely resemble it are taken out.
func namesTheRetiredWord(line string) bool {
	return theRetiredWord.MatchString(notThisRecord.ReplaceAllString(line, ""))
}

// retirementCovers reports whether path, or a directory above it, is ratified.
// The waiver set holds each reason to a standard and reports the entries that
// stopped matching, so a path that moves does not leave a standing permission.
func retirementCovers(t *testing.T, path string) bool {
	for _, prefix := range retired.Subjects() {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return retired.Waived(t, prefix)
		}
	}
	return false
}

// TestAFrozenCriterionExemptsItselfAndNothingElse plants the case the recorded
// criteria exempt, and the case beside it that they must not.
//
// This is the exemption that goes silently permissive: it is derived from a
// corpus, so widening it needs no edit here, and a census that took a whole
// LINE out on a criterion's account would report a clean tree over a planted
// one.
func TestAFrozenCriterionExemptsItselfAndNothingElse(t *testing.T) {
	t.Parallel()

	frozen := frozenCriteria()
	if len(frozen) == 0 {
		t.Skip("no recorded verdict names the retired word, so there is nothing to exempt")
	}
	criterion := frozen[0]

	if namesTheRetiredWord(beyondTheExemptions(criterion)) {
		t.Errorf("a recorded criterion is frozen by its own digest and the census rewrites it:"+
			"\n\t%s", criterion)
	}
	beside := criterion + ` and the column is still person_id`
	if !namesTheRetiredWord(beyondTheExemptions(beside)) {
		t.Error("the census takes a whole line out on a frozen criterion's account, so a " +
			"retired name written beside one walks past it")
	}
}
