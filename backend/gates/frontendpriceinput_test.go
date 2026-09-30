// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The frontend's price pattern accepts exactly what the server's price parser
// and the contract pattern accept, so a value the form lets through is not
// refused for its shape. If the two disagree the form either sends a request
// that can only be refused or refuses a price the sheet would have kept.
//
// The browser's pattern is a declared mirror of the server's domain, and this is
// what makes "mirror" true. It compares both directions over a corpus derived
// from the shape of the domain rather than listed: every whole-digit count
// around the ceiling crossed with every fractional-digit count around its own,
// plus the malformed shapes a decimal can take. And it holds the contract's
// advertised pattern to the same text, so the three spellings are one.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

const (
	frontendPriceInput = "../frontend/src/format/priceinput.ts"
	crmContractPath    = "api/crm.yaml"
)

var (
	tsPriceLiteral = regexp.MustCompile("export const PER_MTOK_PRICE = /(.+)/;")
	// One price field of the set-price request, wherever it sits in the schema.
	contractPriceField = regexp.MustCompile(`(input|output|cache_read|cache_write)_per_mtok: \{ type: string, pattern: '([^']+)'`)
)

func priceCorpus() []string {
	corpus := []string{"", ".", "1.", ".5", " 1", "1 ", "-1", "+1", "1e3", "1/3", "1,5", "0x1", "١٢٣", "1.2.3", "00", "0.000000"}
	for whole := 0; whole <= 14; whole++ {
		for frac := -1; frac <= 8; frac++ {
			s := strings.Repeat("9", whole)
			if frac >= 0 {
				s += "." + strings.Repeat("1", frac)
			}
			corpus = append(corpus, s)
		}
	}
	return corpus
}

func TestTheFrontendPriceInputPatternMatchesTheServersDomain(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendPriceInput)
	if err != nil {
		t.Fatalf("reading the frontend price pattern: %v", err)
	}
	m := tsPriceLiteral.FindStringSubmatch(string(source))
	if m == nil {
		t.Fatalf("%s no longer declares PER_MTOK_PRICE as a regex literal — this gate is reading a shape that is gone", frontendPriceInput)
	}
	pattern, err := regexp.Compile(m[1])
	if err != nil {
		t.Fatalf("the frontend pattern %q is not a pattern Go reads: %v", m[1], err)
	}

	for _, s := range priceCorpus() {
		_, serverErr := ai.UsdPerMTokToMicroUSD("input_per_mtok", s)
		server, browser := serverErr == nil, pattern.MatchString(s)
		if server != browser {
			t.Errorf("%q: the server accepts=%t, the frontend pattern accepts=%t", s, server, browser)
		}
	}

	contract, err := os.ReadFile(crmContractPath)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	fields := contractPriceField.FindAllStringSubmatch(string(contract), -1)
	if len(fields) < 4 {
		t.Fatalf("found %d set-price pattern fields in the contract, want the four buckets — this gate reads nothing it can compare", len(fields))
	}
	for _, f := range fields {
		if f[2] != m[1] {
			t.Errorf("%s_per_mtok is %q in the contract and %q in the frontend", f[1], f[2], m[1])
		}
	}
}
