// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"go/format"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var update = flag.Bool("update", false, "rewrite stylesources_gen.go from the files that own the style rules")

const (
	englishGatePath      = "frontend/src/i18n/copy-style.test.ts"
	vietnameseCensusPath = "frontend/census/rules/copy-style-vi.census.ts"
	vietnamesePagePath   = "docs/reference/ui-copy-style-vi.md"
	snapshotFile         = "stylesources_gen.go"
)

// englishPatternLeads names the code right before each mirrored rule's regex
// literal in the English catalog gate. Spaces in a lead stand for any spacing.
// gatekit:fixture the code before each mirrored rule's regex literal in the English catalog gate
var englishPatternLeads = map[string]string{
	ruleEnContraction: "const contraction =",
	ruleEnSpelling:    "const british =",
	ruleEnWe:          `it("never speaks as we", () => { expect( offenders(`,
	ruleEnI:           `it("says I only inside the onboarding conversation", () => { expect( offenders(`,
	patternEnQuoted:   "const unquoted = (value: string) => value.replace(",
	ruleEnAmpersand:   `const unquoted = (value: string) => value.replace(/“[^”]*”/g, ""); expect(offenders(`,
}

// vietnamesePatternLeads does the same for the Vietnamese census, where a
// pattern is a regex literal or a call of its word().
// gatekit:fixture the code before each mirrored rule's regex literal in the Vietnamese census
var vietnamesePatternLeads = map[string]string{
	ruleViToneOpen:        `"marks the tone of an open oa, oe and uy on the first vowel",`,
	ruleViToneClosed:      `"marks the tone on the second vowel before a final consonant",`,
	patternDash:           `"uses no en dash, em dash, double hyphen or spaced hyphen",`,
	ruleViExclamation:     `matching("uses no exclamation mark",`,
	ruleViToi:             `"says tôi only for the onboarding agent, a consent statement or an English me",`,
	patternToiWe:          `word("tôi"), { text: (value) => prose(value).replace(`,
	patternSaysMe:         `key.startsWith(voices.agentSpeaks) || spokenByTheReader(key) ||`,
	ruleViChungToi:        `"says chúng tôi only as the data controller or in a consent statement",`,
	ruleViChungTa:         `matching("never speaks as chúng ta or mình",`,
	patternReflexive:      `const REFLEXIVE =`,
	ruleViMemberAddress:   `"says quý khách and vui lòng only where an outsider reads",`,
	ruleViOutsiderAddress: `matching("never says bạn where an outsider reads",`,
	patternViQuoted:       `function unquoted(value: string): string { return prose(value).replace(`,
	patternWebAddress:     `const WEB_ADDRESS =`,
	patternFileName:       `const FILE_NAME =`,
}

// vietnamesePins are census code whose lookaround Go cannot read, ported by
// hand in stylerules.go. They stay out of the snapshot, so -update cannot pass
// a census change that the hand port has not followed.
var vietnamesePins = map[string]struct{ lead, code string }{
	"word": {
		`export function word(alternation: string, flags = "iu"): RegExp {`,
		"return new RegExp(`(?<![\\\\p{L}])(?:${alternation})(?![\\\\p{L}])`, flags);",
	},
	ruleViFormal: {
		`"never addresses anyone as quý vị, anh/chị or kính thưa",`,
		`word("quý vị|anh/chị|anh chị(?! em)|kính thưa"),`,
	},
}

// TestTheStyleSnapshotMatchesItsOwners fails when stylesources_gen.go and the
// files it is taken from disagree, whichever of them changed.
func TestTheStyleSnapshotMatchesItsOwners(t *testing.T) {
	t.Parallel()
	root := filepath.Join(gatekit.ModuleRoot(t), "..")
	owners := map[string]string{}
	for _, path := range []string{englishGatePath, vietnameseCensusPath, vietnamesePagePath} {
		full := filepath.Join(root, path)
		if err := gatekit.DeclareInputs(os.Getenv, full); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(filepath.Clean(full))
		if err != nil {
			t.Fatalf("reading a copy style source: %v", err)
		}
		owners[path] = string(body)
	}
	extracted, err := extractSnapshot(owners[englishGatePath], owners[vietnameseCensusPath], owners[vietnamesePagePath])
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderSnapshot(extracted)
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(snapshotFile, rendered, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, difference := range snapshotDifferences(ownerSnapshot, extracted) {
		t.Errorf("%s %s; rewrite it with: go test ./internal/shared/kernel/langcopy/langcopytest -run %s -update",
			snapshotFile, difference, t.Name())
	}
	onDisk, err := os.ReadFile(snapshotFile)
	if err != nil {
		t.Fatal(err)
	}
	if !t.Failed() && !bytes.Equal(onDisk, rendered) {
		t.Errorf("%s was edited by hand; rewrite it with -update", snapshotFile)
	}
}

// extractSnapshot reads the patterns and the Vocabulary table from their owners
// and fails a pin whose census code changed.
func extractSnapshot(gate, census, page string) (styleSnapshot, error) {
	patterns := map[string]jsRegex{}
	for _, owner := range []struct {
		path, text string
		leads      map[string]string
	}{{englishGatePath, gate, englishPatternLeads}, {vietnameseCensusPath, census, vietnamesePatternLeads}} {
		for name, lead := range owner.leads {
			pattern, err := patternAfter(owner.text, lead)
			if err != nil {
				return styleSnapshot{}, fmt.Errorf("%s: the %s pattern: %w", owner.path, name, err)
			}
			patterns[name] = pattern
		}
	}
	for name, pin := range vietnamesePins {
		if err := pinned(census, pin.lead, pin.code); err != nil {
			return styleSnapshot{}, fmt.Errorf("%s: %s is ported by hand: %w", vietnameseCensusPath, name, err)
		}
	}
	words, kept, err := vietnameseVocabulary(page)
	if err != nil {
		return styleSnapshot{}, err
	}
	return styleSnapshot{patterns: patterns, retiredWords: words, keptCompounds: kept}, nil
}

// snapshotDifferences names each pattern and word list on which two snapshots
// disagree.
func snapshotDifferences(held, owned styleSnapshot) []string {
	var differences []string
	for _, name := range slices.Sorted(maps.Keys(owned.patterns)) {
		if got, found := held.patterns[name]; !found {
			differences = append(differences, "lacks the "+name+" pattern")
		} else if got != owned.patterns[name] {
			differences = append(differences, fmt.Sprintf("holds /%s/%s as the %s pattern, and its owner /%s/%s",
				got.source, got.flags, name, owned.patterns[name].source, owned.patterns[name].flags))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(held.patterns)) {
		if _, found := owned.patterns[name]; !found {
			differences = append(differences, "holds a "+name+" pattern that no owner has")
		}
	}
	if !slices.Equal(held.retiredWords, owned.retiredWords) {
		differences = append(differences, fmt.Sprintf("retires %q, and the Vocabulary table %q",
			held.retiredWords, owned.retiredWords))
	}
	if !slices.Equal(held.keptCompounds, owned.keptCompounds) {
		differences = append(differences, fmt.Sprintf("keeps %q, and the Vocabulary table %q",
			held.keptCompounds, owned.keptCompounds))
	}
	return differences
}

// renderSnapshot writes a snapshot as the Go source of stylesources_gen.go.
func renderSnapshot(s styleSnapshot) ([]byte, error) {
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by TestTheStyleSnapshotMatchesItsOwners -update from %s, %s and %s. DO NOT EDIT.\n\n",
		englishGatePath, vietnameseCensusPath, vietnamesePagePath)
	out.WriteString("package langcopytest\n\nvar ownerSnapshot = styleSnapshot{\n\tpatterns: map[string]jsRegex{\n")
	for _, name := range slices.Sorted(maps.Keys(s.patterns)) {
		fmt.Fprintf(&out, "\t\t%q: {%s, %q},\n", name, goString(s.patterns[name].source), s.patterns[name].flags)
	}
	out.WriteString("\t},\n")
	for _, list := range []struct {
		field string
		items []string
	}{{"retiredWords", s.retiredWords}, {"keptCompounds", s.keptCompounds}} {
		fmt.Fprintf(&out, "\t%s: []string{\n", list.field)
		for _, item := range list.items {
			fmt.Fprintf(&out, "\t\t%s,\n", goString(item))
		}
		out.WriteString("\t},\n")
	}
	out.WriteString("}\n")
	return format.Source(out.Bytes())
}

// goString spells a string raw where it can, so a pattern reads as its owner
// writes it.
func goString(s string) string {
	if strconv.CanBackquote(s) {
		return "`" + s + "`"
	}
	return strconv.Quote(s)
}

var (
	// regexLiteral is one JavaScript regex literal and its flags. A character
	// class may hold a slash, so it is read as a unit.
	regexLiteral = regexp.MustCompile(`^/((?:\\.|\[(?:\\.|[^\]\\])*\]|[^/\\\[\n])+)/([a-z]*)`)
	wordCall     = regexp.MustCompile(`^word\(\s*("(?:\\.|[^"\\])*")\s*(?:,\s*"([a-z]*)"\s*)?\)`)
	spacing      = regexp.MustCompile(`\s+`)
)

// leadIn finds the one place a lead stands in a source and answers the code
// after it, with the spacing in between dropped.
func leadIn(source, lead string) (string, error) {
	pieces := strings.Fields(lead)
	for i, piece := range pieces {
		pieces[i] = regexp.QuoteMeta(piece)
	}
	found := regexp.MustCompile(strings.Join(pieces, `\s*`)).FindAllStringIndex(source, -1)
	if len(found) != 1 {
		return "", fmt.Errorf("%q stands there %d times, and must stand once", lead, len(found))
	}
	return strings.TrimLeft(source[found[0][1]:], " \t\n"), nil
}

// patternAfter reads the regex literal or word() call right after a lead, and
// fails one Go cannot compile.
func patternAfter(source, lead string) (jsRegex, error) {
	code, err := leadIn(source, lead)
	if err != nil {
		return jsRegex{}, err
	}
	var pattern jsRegex
	if literal := regexLiteral.FindStringSubmatch(code); literal != nil {
		pattern = jsRegex{literal[1], literal[2]}
	} else {
		call := wordCall.FindStringSubmatch(code)
		if call == nil {
			return jsRegex{}, fmt.Errorf("no regex literal or word() call follows %q", lead)
		}
		alternation, err := strconv.Unquote(call[1])
		if err != nil {
			return jsRegex{}, fmt.Errorf("the word() argument %s is not a plain string: %w", call[1], err)
		}
		pattern = word(alternation, cmp.Or(call[2], "iu"))
	}
	if _, err := pattern.compile(); err != nil {
		return jsRegex{}, err
	}
	return pattern, nil
}

// pinned fails unless code follows lead, spacing aside.
func pinned(source, lead, code string) error {
	after, err := leadIn(source, lead)
	if err != nil {
		return err
	}
	if want := spacing.ReplaceAllString(code, ""); !strings.HasPrefix(spacing.ReplaceAllString(after, ""), want) {
		return fmt.Errorf("the code after %q is no longer %s", lead, code)
	}
	return nil
}

var (
	codeSpan     = regexp.MustCompile("`([^`]+)`")
	keptCompound = regexp.MustCompile(`\(([^()]*) (?:keep theirs|keeps it)\)`)
	listJoin     = regexp.MustCompile(`, | and `)
)

// vietnameseVocabulary reads the Vocabulary table as copy-style-vi.census.ts
// does. A code span in the Never column is retired. A "(… keep theirs)" note
// names the compounds that keep it.
func vietnameseVocabulary(page string) (words, kept []string, err error) {
	rows, err := vocabularyRows(page)
	if err != nil {
		return nil, nil, err
	}
	ticks := 0
	for _, cells := range rows {
		ticks += strings.Count(cells[3], "`")
		for _, m := range codeSpan.FindAllStringSubmatch(cells[3], -1) {
			words = append(words, m[1])
		}
		for _, m := range keptCompound.FindAllStringSubmatch(cells[3], -1) {
			kept = append(kept, listJoin.Split(m[1], -1)...)
		}
	}
	if len(words) == 0 || ticks != 2*len(words) {
		return nil, nil, fmt.Errorf("%s: the Never column of the Vocabulary table holds %d backticks and %d words in code; "+
			"every retired word sits between two", vietnamesePagePath, ticks, len(words))
	}
	if _, _, err := vocabulary(words, kept); err != nil {
		return nil, nil, fmt.Errorf("%s: the Vocabulary table: %w", vietnamesePagePath, err)
	}
	return words, kept, nil
}

// vocabularyRows answers the cells of each body row of the Vocabulary table.
// It fails a table with no Never column, or a row whose cells do not line up.
func vocabularyRows(page string) ([][]string, error) {
	_, section, found := strings.Cut(page, "\n## Vocabulary\n")
	if !found {
		return nil, fmt.Errorf("%s has no Vocabulary section", vietnamesePagePath)
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var rows [][]string
	header := 0
	for line := range strings.SplitSeq(section, "\n") {
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(line, "|")
		switch {
		case header == 0 && len(cells) < 5:
			return nil, fmt.Errorf("%s: the Vocabulary table has no Never column: %q", vietnamesePagePath, line)
		case header == 0:
			header = len(cells)
		case len(cells) != header:
			return nil, fmt.Errorf("%s: the Vocabulary row %q has %d cells and the header %d",
				vietnamesePagePath, line, len(cells), header)
		default:
			rows = append(rows, cells)
		}
	}
	return rows, nil
}

func TestASnapshotThatDiffersFromItsOwnersIsNamed(t *testing.T) {
	t.Parallel()
	owned := styleSnapshot{
		patterns:      map[string]jsRegex{"a": {"x", "i"}, "b": {"y", ""}},
		retiredWords:  []string{"w"},
		keptCompounds: []string{"w k"},
	}
	edited := func(edit func(*styleSnapshot)) styleSnapshot {
		held := styleSnapshot{maps.Clone(owned.patterns), slices.Clone(owned.retiredWords), slices.Clone(owned.keptCompounds)}
		edit(&held)
		return held
	}
	if got := snapshotDifferences(owned, owned); len(got) != 0 {
		t.Errorf("a snapshot equal to its owners was reported: %v", got)
	}
	for name, held := range map[string]styleSnapshot{
		"a pattern the owners dropped": edited(func(s *styleSnapshot) { s.patterns["c"] = jsRegex{"z", ""} }),
		"a pattern the owners added":   edited(func(s *styleSnapshot) { delete(s.patterns, "b") }),
		"a pattern's changed flags":    edited(func(s *styleSnapshot) { s.patterns["a"] = jsRegex{"x", ""} }),
		"a retired word":               edited(func(s *styleSnapshot) { s.retiredWords = append(s.retiredWords, "v") }),
		"a kept compound":              edited(func(s *styleSnapshot) { s.keptCompounds = nil }),
	} {
		if got := snapshotDifferences(held, owned); len(got) != 1 {
			t.Errorf("%s was reported as %v, want one difference", name, got)
		}
	}
}
