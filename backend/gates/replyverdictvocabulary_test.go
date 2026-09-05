// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// The reply verdict is spelled in three places, and they have to agree.
//
// The prompt asks a model for one of three words, the classify engine checks the
// answer against a map, and the database refuses anything outside a CHECK
// constraint. A fourth spelling appearing in any one of them is not a compile
// error and not a test failure anywhere else: the model returns a word the
// engine accepts and the database rejects, so the pass fails per message, at
// run time, on real customer mail.
//
// This is what holds the "cannot drift" claim in captureclassify.go. Without it
// that comment would be one of the false ones AGENTS.md warns about — the kind
// that stops the next author from looking.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The committed head catalog is the authority: it is the schema the migrations
// actually build, so a CHECK read from it is the one the database enforces.
// Reading a named migration file instead would pin this gate to a filename and
// miss a later migration that widened the set — the gate would keep measuring
// history while production moved.
const replyVerdictCatalog = "migrations/testdata/head_catalog.txt"

// Postgres renders an IN list as `= ANY (ARRAY[...])` once the constraint is in
// the catalog, so this reads the catalog's own spelling rather than the
// migration's.
var replyVerdictCheckRE = regexp.MustCompile(
	`activity_reply_verdict_check CHECK .*reply_verdict = ANY \(ARRAY\[([^\]]*)\]`)

func TestTheReplyVerdictVocabularyIsSpelledOnceEverywhere(t *testing.T) {
	t.Parallel()
	fromDB := replyVerdictsFromMigration(t)
	if len(fromDB) != 3 {
		t.Fatalf("the migration's CHECK names %d verdicts (%v), and this gate was written against three — "+
			"if the vocabulary genuinely changed, every reader below changed with it and this number moves too",
			len(fromDB), fromDB)
	}

	fromConstants := replyVerdictConstants(t)
	if !equalSets(fromConstants, fromDB) {
		t.Errorf("the module's constants spell %v and the database CHECK allows %v.\n"+
			"\tA constant outside the CHECK stores a verdict the database refuses, per message, at run time.",
			fromConstants, fromDB)
	}

	// The history table's CHECK is a second copy of the same closed set, and a
	// migration widening one and not the other leaves a correction the activity
	// accepts and the history refuses — the write fails halfway through a
	// transaction that has already updated the row.
	if history := replyVerdictsFromHistoryCheck(t); !equalSets(history, fromDB) {
		t.Errorf("the history CHECK allows %v and the activity CHECK allows %v — "+
			"a correction the row accepts would be refused by the record of it", history, fromDB)
	}

	engine, err := os.ReadFile("internal/compose/captureclassify.go")
	if err != nil {
		t.Fatalf("reading the classify engine: %v", err)
	}

	// The engine's map must be built FROM the module's constants, and this reads
	// the MAP rather than the file: a whole-file search is satisfied by a mention
	// in a comment or an error string, so it would keep passing after the map
	// itself drifted to literals — the exact drift this gate exists to catch.
	mapBody := between(string(engine), "var replyVerdicts = map[string]bool{", "}")
	if mapBody == "" {
		t.Fatal("no `var replyVerdicts = map[string]bool{` in the classify engine — " +
			"the map this gate measures has been renamed or removed")
	}
	for _, want := range []string{
		"activities.ReplyVerdictPositive",
		"activities.ReplyVerdictNegative",
		"activities.ReplyVerdictNeutral",
	} {
		if !strings.Contains(mapBody, want) {
			t.Errorf("the verdict map does not reference %s.\n"+
				"\tBuild it from the module's constants; a literal here is a second spelling "+
				"that agrees until somebody edits one side.", want)
		}
	}

	// The response schema's enum is the third copy: a word the database accepts
	// and the schema omits is one no model may ever return, so the column could
	// hold a value nothing can produce.
	schemaEnum := between(string(engine), `"reply": schema.Enum(`, ")")
	if schemaEnum == "" {
		t.Fatal(`no "reply": schema.Enum(...) in the classify engine — ` +
			"the response schema this gate measures has moved")
	}
	// And the PROMPT has to offer every word too. Read from the system prompt
	// alone, for the same reason the map is: a word appearing anywhere in the
	// file would satisfy a whole-file search after the prompt stopped naming it.
	systemPrompt := between(string(engine), "const classifySystem = `", "`")
	if systemPrompt == "" {
		t.Fatal("no `const classifySystem` in the classify engine — the prompt this gate measures has moved")
	}
	for _, verdict := range fromDB {
		if !strings.Contains(schemaEnum, `"`+verdict+`"`) {
			t.Errorf("the response schema's reply enum omits %q, which the database accepts.\n"+
				"\tA verdict no schema admits is a column value no model can ever return.", verdict)
		}
		if !strings.Contains(systemPrompt, `"`+verdict+`"`) {
			t.Errorf("the classify prompt never offers %q, which the database accepts.\n"+
				"\tA verdict nothing asks for is a column value that can never occur.", verdict)
		}
	}
}

// replyVerdictsFromHistoryCheck reads the closed set off the history table's own
// constraint, so the two copies are compared rather than assumed equal.
func replyVerdictsFromHistoryCheck(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(replyVerdictCatalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	re := regexp.MustCompile(`activity_reply_verdict_history_verdict CHECK .*verdict = ANY \(ARRAY\[([^\]]*)\]`)
	match := re.FindSubmatch(raw)
	if match == nil {
		t.Fatalf("no activity_reply_verdict_history_verdict constraint in %s", replyVerdictCatalog)
	}
	return splitCatalogTokens(string(match[1]))
}

// replyVerdictsFromMigration reads the closed set off the CHECK constraint that
// enforces it.
func replyVerdictsFromMigration(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(replyVerdictCatalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	match := replyVerdictCheckRE.FindSubmatch(raw)
	if match == nil {
		t.Fatalf("no activity_reply_verdict_check in %s — the constraint this gate "+
			"measures everything else against has moved or been renamed", replyVerdictCatalog)
	}
	return splitCatalogTokens(string(match[1]))
}

// splitCatalogTokens turns a catalog ARRAY[...] body into its bare words.
//
// TrimSuffix and not Trim: a cutset removes ANY of its characters from either
// end, so trimming "'::text" off 'positive' also eats the final e, and the gate
// then compares words that were never in the catalog.
func splitCatalogTokens(body string) []string {
	var out []string
	for _, part := range strings.Split(body, ",") {
		token := strings.TrimSuffix(strings.TrimSpace(part), "::text")
		out = append(out, strings.Trim(token, "'"))
	}
	sort.Strings(out)
	return out
}

// replyVerdictConstants reads the module's exported spellings.
func replyVerdictConstants(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("internal/modules/activities/replyverdict.go")
	if err != nil {
		t.Fatalf("reading the reply-verdict store: %v", err)
	}
	re := regexp.MustCompile(`ReplyVerdict(?:Positive|Negative|Neutral)\s*=\s*"([^"]+)"`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func equalSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
