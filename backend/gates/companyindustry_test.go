// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func camelIndustryKey(code string) string {
	parts := strings.Split(code, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

func readFrontendFile(t *testing.T, elems ...string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(append([]string{repoRoot, "frontend", "src"}, elems...)...))
	if err != nil {
		t.Fatalf("reading the frontend file: %v", err)
	}
	return string(raw)
}

// The industry list lives in the contract, the vocabulary owner in the contacts
// module and its wording in the frontend. The contract is the corpus: a code
// any of the others lacks, or holds twice, fails here.
func TestTheIndustryListIsOneSetInTheContractTheOwnerAndTheClient(t *testing.T) {
	t.Parallel()
	doc := loadContractDocument(t)
	contract := contractEnum(t, doc, "Company", "industry")
	if len(contract) < 20 || len(contract) > 40 {
		t.Fatalf("the contract lists %d industries, want between 20 and 40", len(contract))
	}
	owner := contacts.IndustryCodes()
	if len(owner) != len(contract) {
		t.Errorf("the owner holds %d codes and the contract %d", len(owner), len(contract))
	}
	for _, code := range owner {
		if !contract[code] {
			t.Errorf("the owner holds %q and the contract does not admit it", code)
		}
	}

	client := regexp.MustCompile(`(?m)^  "(\w+)",$`)
	module := readFrontendFile(t, "format", "industry.ts")
	listed := map[string]bool{}
	start := strings.Index(module, "INDUSTRY_CODES")
	if start < 0 {
		t.Fatal("format/industry.ts does not declare INDUSTRY_CODES")
	}
	for _, match := range client.FindAllStringSubmatch(module[start:], -1) {
		listed[match[1]] = true
	}
	for code := range contract {
		if !listed[code] {
			t.Errorf("format/industry.ts does not list %q", code)
		}
	}
	for code := range listed {
		if !contract[code] {
			t.Errorf("format/industry.ts lists %q and the contract does not admit it", code)
		}
	}
}

// Each language words every code the same way on both sides of the wire.
// A label read in a prompt and a label read on a screen are one string.
func TestEveryIndustryCodeIsWordedTheSameInGoAndInEveryClientCatalog(t *testing.T) {
	t.Parallel()
	for lang, file := range map[textlang.Lang]string{
		textlang.English: "en.ts", textlang.German: "de.ts", textlang.Vietnamese: "vi.ts",
	} {
		catalog := readFrontendFile(t, "i18n", file)
		for _, code := range contacts.IndustryCodes() {
			want := contacts.IndustryLabel(code, lang)
			pattern := regexp.MustCompile(`"industry\.` + camelIndustryKey(code) + `":\s*"([^"]*)"`)
			match := pattern.FindStringSubmatch(catalog)
			if match == nil {
				t.Errorf("%s has no industry.%s", file, camelIndustryKey(code))
				continue
			}
			if match[1] != want {
				t.Errorf("%s words %s as %q and the owner as %q", file, code, match[1], want)
			}
		}
	}
}

// Every enum the contract declares for an industry or a size band, on a schema
// property or a query parameter, is the company's own list. A copy is found by
// its name, so a new one that drifts fails here without being listed.
func TestEveryIndustryAndSizeBandEnumIsTheCompanys(t *testing.T) {
	t.Parallel()
	doc := loadContractDocument(t)
	want := map[string]map[string]bool{
		"industry":  contractEnum(t, doc, "Company", "industry"),
		"size_band": contractEnum(t, doc, "Company", "size_band"),
	}
	census := map[string][]string{}
	walkVocabularyCopies(doc, "", func(where, name string, schema map[string]any) {
		raw, closed := schema["enum"].([]any)
		if !closed {
			return
		}
		census[name] = append(census[name], where)
		admitted := map[string]bool{}
		for _, value := range enumStrings(t, map[string]any{"enum": raw}, where) {
			admitted[value] = true
		}
		if !maps.Equal(admitted, want[name]) {
			t.Errorf("%s admits %v, want the company's %s list %v", where, admitted, name, want[name])
		}
	})
	t.Logf("industry enums (%d): %v", len(census["industry"]), census["industry"])
	t.Logf("size_band enums (%d): %v", len(census["size_band"]), census["size_band"])
	// The copies the contract holds today; a walk that finds fewer has stopped reaching some.
	if len(census["industry"]) < 5 || len(census["size_band"]) < 6 {
		t.Errorf("the walk found %v, fewer copies than the contract holds, so it compared too little", census)
	}
}

// walkVocabularyCopies calls visit with the schema of every property and query
// parameter named industry or size_band, anywhere under node.
//
//craft:ignore naked-any the contract is read as untyped YAML nodes, as every reader in this file reads it
func walkVocabularyCopies(node any, where string, visit func(where, name string, schema map[string]any)) {
	switch typed := node.(type) {
	case map[string]any:
		if properties, ok := typed["properties"].(map[string]any); ok {
			for _, name := range []string{"industry", "size_band"} {
				if schema, ok := properties[name].(map[string]any); ok {
					visit(where+"."+name, name, schema)
				}
			}
		}
		if name, ok := typed["name"].(string); ok && typed["in"] == "query" && (name == "industry" || name == "size_band") {
			if schema, ok := typed["schema"].(map[string]any); ok {
				visit(where+"?"+name, name, schema)
			}
		}
		for key, child := range typed {
			walkVocabularyCopies(child, where+"/"+key, visit)
		}
	case []any:
		for _, child := range typed {
			walkVocabularyCopies(child, where, visit)
		}
	}
}
