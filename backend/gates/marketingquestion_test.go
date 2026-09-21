// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// The question a grant is bound to is the question the screen asks.
//
// A subscription consent's proof now names a published row: the subject is
// recorded as having agreed to the wording this installation published, in the
// language their link was minted in. That claim is only true while the
// published wording and the wording on the screen are the same sentence.
//
// They live in two catalogs — backend/internal/platform/mailcopy for the
// published one, frontend/src/i18n for the rendered one — and nothing in either
// build makes them agree. Edit the screen's string alone and every grant after
// it records somebody agreeing to a sentence they were not shown, which is the
// defect the binding exists to end, arriving from the other side.
//
// WHEN THE QUESTION CHANGES: edit both catalogs, and bump
// marketingQuestionVersion in consent/marketingquestion.go. The bump is what
// tells a reader the proposition moved; the old version stays published for the
// grants that named it.
//
// THE PROPER FIX, when somebody has the appetite, is for the page to RENDER the
// published wording rather than its own copy — one source, nothing to drift.
// This gate is what makes the two-catalog arrangement survivable until then.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Where each catalog keeps the question.
const (
	serverCopyFile   = "internal/platform/mailcopy/catalog.go"
	frontendCopyRoot = "../frontend/src/i18n/"
)

// theQuestionsSharedFields are the screen's keys and the server fields that
// must say the same thing, in the order the published body joins them.
var theQuestionsSharedFields = []struct {
	screenKey   string
	serverField string
}{
	{"confirm.marketing.ask", "ConfirmMarketingAsk"},
	{"confirm.marketing.yes", "ConfirmMarketingYes"},
	{"confirm.marketing.no", "ConfirmMarketingNo"},
	// The dedicated subscription link's page asks a different question, and a
	// grant through that door binds it. It drifts the same way.
	{"confirm.subscription.ask", "ConfirmSubscriptionAsk"},
	{"confirm.subscription.confirm", "ConfirmSubscriptionConfirm"},
}

// theQuestionsLanguages maps each frontend catalog onto the position its
// translation occupies in the server's three-argument line() call.
var theQuestionsLanguages = []struct {
	file     string
	position int
}{
	{"en.ts", 0},
	{"de.ts", 1},
	{"vi.ts", 2},
}

func TestThePublishedQuestionIsTheOneTheScreenAsks(t *testing.T) {
	t.Parallel()

	server, err := os.ReadFile(serverCopyFile)
	if err != nil {
		t.Fatalf("reading the published copy: %v", err)
	}
	for _, field := range theQuestionsSharedFields {
		published := publishedRenderings(t, string(server), field.serverField)
		for _, language := range theQuestionsLanguages {
			shown := screenRendering(t, language.file, field.screenKey)
			want := published[language.position]
			if shown != want {
				t.Errorf("%s in %s disagrees with %s in %s:\n"+
					"  screen:    %q\n"+
					"  published: %q\n"+
					"A grant records the published sentence as what the subject agreed to, so a "+
					"screen showing a different one records an agreement to words nobody saw.",
					field.screenKey, language.file, field.serverField, serverCopyFile, shown, want)
			}
		}
	}
}

// publishedRenderings pulls one field's three translations out of its line()
// call. Read from source rather than by importing the package, because a gate
// that linked the catalog would be satisfied by a build that compiles.
func publishedRenderings(t *testing.T, source, field string) [3]string {
	t.Helper()
	call := regexp.MustCompile(
		`return &c\.` + regexp.QuoteMeta(field) + ` \}\,\s*((?:\s*"(?:[^"\\]|\\.)*",?\n?)+)\)`)
	match := call.FindStringSubmatch(source)
	if match == nil {
		t.Fatalf("no line() call for %s in %s: this gate is not reading what it thinks it is",
			field, serverCopyFile)
	}
	literals := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(match[1], -1)
	if len(literals) != 3 {
		t.Fatalf("%s has %d rendering(s), want the three languages", field, len(literals))
	}
	var out [3]string
	for i, literal := range literals {
		out[i] = unquoted(t, literal[1])
	}
	return out
}

// screenRendering pulls one key's value out of a frontend catalog.
func screenRendering(t *testing.T, file, key string) string {
	t.Helper()
	raw, err := os.ReadFile(frontendCopyRoot + file)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	// The value may sit on the same line as the key or wrap onto the next, and
	// a long one is split across several adjacent literals.
	entry := regexp.MustCompile(
		regexp.QuoteMeta(`"`+key+`":`) + `\s*((?:"(?:[^"\\]|\\.)*"\s*\+?\s*)+)`)
	// EVERY definition, not the first. A key defined twice is legal
	// TypeScript and the LAST one wins at runtime, so a gate reading the first
	// would compare a sentence the screen does not show — which is how a
	// second, disagreeing definition would have slipped past this check.
	matches := entry.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatalf("%s is not in %s: the screen no longer asks the question this gate compares",
			key, file)
	}
	if len(matches) > 1 {
		t.Errorf("%s is defined %d times in %s. The last wins at runtime, so the others are "+
			"dead and one of them is what this gate would otherwise have compared.",
			key, len(matches), file)
	}
	var built strings.Builder
	for _, literal := range regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`).
		FindAllStringSubmatch(matches[len(matches)-1][1], -1) {
		built.WriteString(unquoted(t, literal[1]))
	}
	return built.String()
}

// unquoted turns one source literal's body into the string it denotes. Both
// catalogs write Go-style escapes — the frontend's are \uXXXX sequences the
// formatter produced — so one unquoter serves both.
func unquoted(t *testing.T, body string) string {
	t.Helper()
	out, err := strconv.Unquote(`"` + body + `"`)
	if err != nil {
		t.Fatalf("reading the literal %q: %v", body, err)
	}
	return out
}
