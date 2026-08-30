// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The weekly message's labels are the weekly PANEL's labels.
//
// The message is a summary of numbers the reader can also see on screen, and
// its whole subject is their own week. Two translations of "Promised,
// delivered" is how the mail comes to call something by a name the panel does
// not — in the one message where the reader has the other version open in
// another tab.
//
// So the Go catalog carries the frontend catalog's strings, character for
// character, and this compares them in all three languages. What it does NOT
// hold is the transactional copy: a password link and an invitation have no
// screen behind them, so there is nothing to agree with.
//
// The frontend catalog is read as TEXT rather than parsed, and that is a real
// limit: these files are TypeScript, and reading them properly would mean a
// second parser for the frontend to keep in step. The reader below refuses when
// it cannot find a key — an absent key would otherwise compare nothing and pass
// — so the failure mode is a loud one.

import (
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// mailLabelPairs are the Go fields that must equal a frontend key, per field.
//
// Only the labels the two surfaces SHARE. The message says things the panel
// does not — a subject line, a closing link — and those are the mail's own.
var mailLabelPairs = []struct {
	key   string
	field func(mailcopy.Copy) string
}{
	{"home.weekly.promised", func(c mailcopy.Copy) string { return c.WeeklyPromised }},
	// The count template, with the panel's {done}/{due} placeholders read as
	// the %d pair the mail formats with: the ORDER is the localised part, and
	// a language that puts the total first would otherwise read backwards.
	{"home.weekly.ofDue", func(c mailcopy.Copy) string { return placeholders(c.WeeklyOfDue) }},
	{"home.weekly.dealsWon", func(c mailcopy.Copy) string { return c.WeeklyDealsWon }},
	{"home.weekly.dealsLost", func(c mailcopy.Copy) string { return c.WeeklyDealsLost }},
	{"home.weekly.dealsMoved", func(c mailcopy.Copy) string { return c.WeeklyMoved }},
	{"home.weekly.decided", func(c mailcopy.Copy) string { return c.WeeklyDecided }},
	{"home.weekly.queueWorked", func(c mailcopy.Copy) string { return c.WeeklyQueue }},
	{"home.weekly.carriedOver", func(c mailcopy.Copy) string { return c.WeeklyCarried }},
	{"home.weekly.outcome.won", func(c mailcopy.Copy) string { return c.WeeklyOutcomeWon }},
	{"home.weekly.outcome.lost", func(c mailcopy.Copy) string { return c.WeeklyOutcomeLost }},
	{"home.weekly.outcome.moved", func(c mailcopy.Copy) string { return c.WeeklyOutcomeMoved }},
}

// mailLabelLanguages reads the languages the contract admits, so adding a
// fourth to the enum compares that one too rather than leaving it unchecked —
// which is the drift this gate exists to catch, in the one language nobody had
// thought to list here.
func mailLabelLanguages(t *testing.T) []string {
	t.Helper()
	document, err := os.ReadFile(mailLanguageContract)
	if err != nil {
		t.Fatal(err)
	}
	declared := regexp.MustCompile(`base_language:\s*\n\s*type: string\s*\n\s*enum: \[([^\]]*)\]`).
		FindStringSubmatch(string(document))
	if declared == nil {
		t.Fatal("the contract's base_language enum could not be read: this gate compares the mail catalog " +
			"against the frontend in each language it admits, so it now certifies nothing")
	}
	var out []string
	for _, raw := range strings.Split(declared[1], ",") {
		if language := strings.TrimSpace(raw); language != "" {
			out = append(out, language)
		}
	}
	if len(out) < 2 {
		t.Fatalf("the contract admits %d language(s), and this tree ships three: the reader has stopped "+
			"matching rather than the contract having shrunk", len(out))
	}
	return out
}

// TestEveryMailLabelMatchesTheScreenThatShowsIt is the parity check.
func TestEveryMailLabelMatchesTheScreenThatShowsIt(t *testing.T) {
	t.Parallel()
	for _, language := range mailLabelLanguages(t) {
		t.Run(language, func(t *testing.T) {
			t.Parallel()
			screen := frontendCatalog(t, language)
			mail := mailcopy.For(language)
			for _, pair := range mailLabelPairs {
				onScreen, found := screen[pair.key]
				if !found {
					t.Errorf("the %s catalog declares no %s, so this gate compared nothing for it — either the "+
						"key was renamed and this list must follow, or the panel lost a label the message still sends",
						language, pair.key)
					continue
				}
				if got := pair.field(mail); got != onScreen {
					t.Errorf("the %s message says %q where the panel says %q (%s) — a reader with both open is "+
						"being told their own week in two vocabularies",
						language, got, onScreen, pair.key)
				}
			}
		})
	}
}

// frontendCatalog reads one language's message keys out of its own file.
//
// Only the SINGLE-LINE entries: every key this gate compares is a short label
// that fits on one, and a reader that tried to stitch wrapped strings would be
// re-implementing the language's own escaping to do it. A key that is wrapped
// therefore reads as absent, which the caller reports rather than skipping.
func frontendCatalog(t *testing.T, language string) map[string]string {
	t.Helper()
	source, err := os.ReadFile("../frontend/src/i18n/" + language + ".ts")
	if err != nil {
		t.Fatalf("reading the %s catalog: %v", language, err)
	}
	entry := regexp.MustCompile(`(?m)^\s*"([^"]+)":\s*(".*")\s*,\s*$`)
	out := map[string]string{}
	for _, m := range entry.FindAllStringSubmatch(string(source), -1) {
		if text, err := strconv.Unquote(m[2]); err == nil {
			out[m[1]] = text
		}
	}
	if len(out) == 0 {
		t.Fatalf("the %s catalog yielded no keys at all: this reader has stopped matching, and an empty "+
			"comparison passes", language)
	}
	return out
}

// TestTheMailCatalogSpeaksEveryLanguageTheContractAdmits holds the OTHER half:
// a language the contract admits and this catalog has no copy for is sent in
// the fallback, silently, and nobody finds out until a customer reads English
// mail on a German installation.
func TestTheMailCatalogSpeaksEveryLanguageTheContractAdmits(t *testing.T) {
	t.Parallel()
	for _, language := range mailLabelLanguages(t) {
		if mailcopy.For(language).WeeklyPromised == mailcopy.For("zz-not-a-language").WeeklyPromised &&
			language != string(mailcopy.Fallback) {
			t.Errorf("the contract admits base_language %q and the mail catalog has no copy for it, so an "+
				"installation set to it is sent %s mail without anything saying so",
				language, mailcopy.Fallback)
			continue
		}
		// EVERY FIELD, not merely the entry. A keyed struct literal does not
		// require them all, and an omitted one is the empty string — for a
		// subject, a message the reader's client files as blank. Nothing about
		// the type says so, which is why this is here.
		written := reflect.ValueOf(mailcopy.For(language))
		for i := range written.NumField() {
			if written.Field(i).Kind() == reflect.String && written.Field(i).String() == "" {
				t.Errorf("the %s copy leaves %s empty, and an empty line reaches a reader as a blank subject "+
					"or a missing sentence — a keyed struct literal does not require it, so nothing else says so",
					language, written.Type().Field(i).Name)
			}
		}
	}
}

// mailLanguageContract is where the admitted languages are published.
const mailLanguageContract = "api/crm.yaml"

// placeholders rewrites a mail template's %d pair as the panel's own named
// placeholders, in order, so the two spellings of one template can be compared
// as text.
//
// The ORDER is the localised part — a language that named the total first would
// read backwards — and it is the half a comparison of the surrounding words
// alone would miss.
func placeholders(template string) string {
	for _, name := range []string{"{done}", "{due}"} {
		template = strings.Replace(template, "%d", name, 1)
	}
	return template
}
