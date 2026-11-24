// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The limits a screen's own pre-send check states in a refusal sentence equal
// the server's. Only those copies are listed; a copy gone, doubled or changed fails.

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// frontendLimit is one limit a frontend file spells, the pattern that reads it
// and the owner's value it must equal.
type frontendLimit struct {
	file  string
	name  string
	read  *regexp.Regexp
	owner func(t *testing.T, contract map[string]any) []string
}

func TestTheFrontendsCopiesOfServerLimitsMatchTheirOwners(t *testing.T) {
	t.Parallel()
	contract := loadContractDocument(t)
	limits := []frontendLimit{
		{
			file: "../frontend/src/components/create.fields.ts", name: "EARLIEST_DAY",
			read:  regexp.MustCompile(`\bEARLIEST_DAY\s*=\s*"([^"]*)"`),
			owner: fixed(datasource.AcceptedInstantMin),
		},
		{
			file: "../frontend/src/components/create.fields.ts", name: "LATEST_DAY",
			read:  regexp.MustCompile(`\bLATEST_DAY\s*=\s*"([^"]*)"`),
			owner: fixed(datasource.AcceptedInstantMax),
		},
		{
			file: "../frontend/src/lib/installationcompany.ts", name: "COMPANY_NAME_MAX",
			read: regexp.MustCompile(`\bCOMPANY_NAME_MAX\s*=\s*(\d+)`),
			// The anchor company's write refuses a name past this.
			owner: fixed(strconv.Itoa(values.TextLabelCharacters)),
		},
		{
			file: "../frontend/src/features/settings/rates.tsx", name: "RATE_DIGITS",
			read:  regexp.MustCompile(`\bRATE_DIGITS\s*=\s*(\d+)`),
			owner: fxRateDigits,
		},
	}
	sources := map[string]string{}
	for _, limit := range limits {
		source, read := sources[limit.file]
		if !read {
			source = tsComment.ReplaceAllString(readFile(t, limit.file), " ")
			sources[limit.file] = source
		}
		found := limit.read.FindAllStringSubmatch(source, -1)
		if len(found) != 1 {
			t.Errorf("%s spells %s %d times, want once: this gate reads a shape that changed", limit.file, limit.name, len(found))
			continue
		}
		for _, want := range limit.owner(t, contract) {
			if found[0][1] != want {
				t.Errorf("%s holds %s = %s, and its owner says %s: the screen and the server refuse different values",
					limit.file, limit.name, found[0][1], want)
			}
		}
	}
}

func fixed(value string) func(*testing.T, map[string]any) []string {
	return func(*testing.T, map[string]any) []string { return []string{value} }
}

// fxRateDigits reads the whole and fractional digit counts out of the rate's
// pattern. Both are the one limit the screen holds.
func fxRateDigits(t *testing.T, contract map[string]any) []string {
	t.Helper()
	pattern, ok := bodyProperty(t, contract, "/fx-rates", "post", "rate")["pattern"].(string)
	if !ok {
		t.Fatal("post /fx-rates: rate declares no pattern")
	}
	counts := regexp.MustCompile(`\{1,(\d+)\}`).FindAllStringSubmatch(pattern, -1)
	if len(counts) != 2 {
		t.Fatalf("post /fx-rates: rate pattern %q has %d digit counts, want the whole and the fraction", pattern, len(counts))
	}
	return []string{counts[0][1], counts[1][1]}
}

func bodyProperty(t *testing.T, contract map[string]any, path, method, property string) map[string]any {
	t.Helper()
	operation := descendContract(t, contract, "paths", path, method)
	schema := descendContract(t, operation, "requestBody", "content", "application/json", "schema")
	if ref, ok := schema["$ref"].(string); ok {
		schema = descendContract(t, contract, "components", "schemas", ref[len("#/components/schemas/"):])
	}
	prop, ok := descendContract(t, schema, "properties")[property].(map[string]any)
	if !ok {
		t.Fatalf("%s %s: the body has no %s", method, path, property)
	}
	return prop
}
