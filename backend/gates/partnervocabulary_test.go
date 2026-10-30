// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The partner kinds and stage meanings are spelled in the contract and again
// in places no generated type reaches: the MCP tool's input schema, the
// migration CHECK on partner_kinds, the frontend label maps, and the refusal
// messages that name the allowed words. Each copy is held equal to the
// contract both ways, so a kind added to one and not the others fails here
// instead of reaching a caller as a refusal of a valid word.
//
// The approval decoder and the store checks call the generated Valid(), so
// they are not copies and are not listed.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// quotedWords reads every quoted word in a list such as `"a","b"` or `'a', 'b'`.
var quotedWords = regexp.MustCompile(`["']([a-z_]+)["']`)

func wordsIn(list string) []string {
	var out []string
	for _, m := range quotedWords.FindAllStringSubmatch(list, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// vocabularyCopy is one place a contract enum is spelled again: the file, the
// pattern whose first group holds the list, and how the list is read.
type vocabularyCopy struct {
	file    string
	pattern *regexp.Regexp
	read    func(string) []string
}

func TestThePartnerVocabulariesMatchTheContractEverywhere(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	for schema, copies := range map[string][]vocabularyCopy{
		"PartnerKind": {
			{"internal/modules/agents/tools_partnersetup.go", regexp.MustCompile(`"partner_kinds":\{[^}]*"enum":\[([^\]]*)\]`), wordsIn},
			{"../frontend/src/lib/partnerstages.queries.ts", regexp.MustCompile(`(?s)KIND_LABELS: Record<PartnerKind, MessageKey> = \{(.*?)\};`), mapKeys},
		},
		"PartnerStageMeaning": {
			{"../frontend/src/lib/partnerstages.queries.ts", regexp.MustCompile(`(?s)MEANING_LABELS: Record<\s*PartnerStageMeaning,\s*MessageKey\s*> = \{(.*?)\};`), mapKeys},
		},
	} {
		want := wordsIn(`"` + strings.Join(crmYAMLNamedEnum(t, schema), `","`) + `"`)
		for _, c := range copies {
			body, err := os.ReadFile(filepath.Join(root, c.file))
			if err != nil {
				t.Fatalf("reading %s: %v", c.file, err)
			}
			m := c.pattern.FindSubmatch(body)
			if m == nil {
				t.Errorf("%s no longer spells %s where this gate looks; move the pattern with it", c.file, schema)
				continue
			}
			if got := c.read(string(m[1])); !slices.Equal(got, want) {
				t.Errorf("%s spells %s as %v; the contract says %v", c.file, schema, got, want)
			}
		}
	}
	checkKindsCheckConstraint(t, root)
	checkVocabularyMessages(t, root)
}

// mapKeys reads the keys of a TypeScript object literal, one `key:` per entry.
func mapKeys(literal string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^\s*([a-z_]+):`).FindAllStringSubmatch(literal, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// checkKindsCheckConstraint holds the newest partner_kinds CHECK in the core
// migrations to the contract's kinds.
func checkKindsCheckConstraint(t *testing.T, root string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "migrations", "core", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	check := regexp.MustCompile(`partner_kinds <@ ARRAY\[([^\]]*)\]`)
	var newest []string
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if m := check.FindSubmatch(body); m != nil {
			newest = wordsIn(string(m[1]))
		}
	}
	want := wordsIn(`"` + strings.Join(crmYAMLNamedEnum(t, "PartnerKind"), `","`) + `"`)
	if !slices.Equal(newest, want) {
		t.Errorf("the partner_kinds CHECK admits %v; the contract says %v", newest, want)
	}
}

// checkVocabularyMessages finds every string in the partner code that starts
// listing a vocabulary and requires it to name the whole of it.
func checkVocabularyMessages(t *testing.T, root string) {
	t.Helper()
	sentences := map[string]*regexp.Regexp{
		"PartnerKind":         regexp.MustCompile(`"[^"\n]*hosting, consulting, strategic[^"\n]*"`),
		"PartnerStageMeaning": regexp.MustCompile(`"[^"\n]*candidate, signed[^"\n]*"`),
	}
	for _, dir := range []string{"internal/modules/contacts", "internal/modules/agents", "internal/compose"} {
		files, err := filepath.Glob(filepath.Join(root, dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for schema, sentence := range sentences {
				for _, literal := range sentence.FindAllString(string(body), -1) {
					for _, value := range crmYAMLNamedEnum(t, schema) {
						if !strings.Contains(literal, value) {
							rel, _ := filepath.Rel(root, file)
							t.Errorf("%s lists %s without %q: %s", rel, schema, value, literal)
						}
					}
				}
			}
		}
	}
}
