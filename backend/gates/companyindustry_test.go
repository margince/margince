// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
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
	contract := contractEnum(t, loadContractDocument(t), "Company", "industry")
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
