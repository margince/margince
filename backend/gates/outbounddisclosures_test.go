// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every outbound identity says what it discloses.
//
// docs/reference/ai-egress.md is cited by the works-agreement template (§7) and
// by the DPIA as the complete answer to "what leaves this machine". It was
// generated from the AI routing table, so it answered for model and embedding
// providers and for nothing else — while this installation can also be
// configured to send a contact's name to a search API, a postal address to a
// geocoder, and a URL to whatever host it names.
//
// A document cited as exhaustive and not exhaustive is worse than none: the
// operator who writes their own processing record from it writes something
// untrue, and nothing here tells them.
//
// So the page is generated from outbound.Disclosures(), and this holds that
// against the identities themselves. The corpus is the SAME parse
// declaredIdentities uses for the use-count census one file over — a token
// added without a disclosure is a call this installation makes and cannot
// answer for, and it fails here on the day it lands rather than in front of a
// works council.
//
// WHAT THIS CANNOT SEE: whether a Category is TRUE. That a search query
// carries a name is a judgement about the caller, and no test can read it off
// the type — a URL is a string whether it names a company's homepage or somebody's own. What
// it can hold is that somebody wrote one down, beside the constant, where the
// reviewer adding a caller is looking.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestEveryOutboundIdentitySaysWhatItDiscloses(t *testing.T) {
	t.Parallel()

	// Joined on the constant's VALUE ("margince-search"), not its Go name: the
	// value is what a remote operator reads in a log or matches in robots.txt,
	// and it is what Disclosures() carries by referencing the constant itself.
	identities := identityValues(t)
	if len(identities) == 0 {
		t.Fatal("no outbound identity constants parsed, so this census proved nothing — " +
			"either the file moved or the constant shape changed")
	}

	declared := map[string]outbound.Disclosure{}
	for _, d := range outbound.Disclosures() {
		if _, repeated := declared[d.Product]; repeated {
			t.Errorf("two disclosures name %q; one identity is one answer", d.Product)
		}
		declared[d.Product] = d
	}

	for _, name := range sortedKeys(identities) {
		value := identities[name]
		d, found := declared[value]
		if !found {
			t.Errorf("%sProduct (%q) has no entry in outbound.Disclosures().\n\tAn identity exists "+
				"because a call goes out under it, and docs/reference/ai-egress.md is generated from "+
				"that list — so a token with no entry is a disclosure the works agreement and the DPIA "+
				"both claim to cover and neither names.", name, value)
			continue
		}
		if d.Personal && d.Category == "" {
			t.Errorf("%sProduct discloses personal data and names no category.\n\tThe category is what "+
				"a reader of the works agreement acts on; \"personal data\" alone tells them nothing "+
				"about what the receiving party sees.", name)
		}
		if !d.Personal && d.Category != "" {
			t.Errorf("%sProduct names a data category and is not marked personal — one of the two is "+
				"wrong, and the page would print a category under a heading that says there is none.", name)
		}
		if d.Endpoint == "" {
			t.Errorf("%sProduct names no endpoint, so the page cannot say who receives it", name)
		}
	}

	known := map[string]bool{}
	for _, value := range identities {
		known[value] = true
	}
	for value := range declared {
		if !known[value] {
			t.Errorf("outbound.Disclosures() answers for %q, which is not an identity this product "+
				"declares.\n\tA disclosure for a call nobody makes reads as a promise being kept that "+
				"was never at risk.", value)
		}
	}
}

// identityValues reads each identity constant's name and its literal value from
// the same file declaredIdentities parses, so the two censuses over this package
// cannot come to disagree about what it declares.
func identityValues(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), outboundIdentities, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", outboundIdentities, err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Values) != len(value.Names) {
				continue
			}
			for i, name := range value.Names {
				found := identityConstant.FindStringSubmatch(name.Name)
				if found == nil {
					continue
				}
				text, isText := gatekit.LiteralText(value.Values[i])
				if !isText {
					continue
				}
				out[found[1]] = text
			}
		}
	}
	return out
}
