// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Org-name matching for Chinese, Japanese and Korean, where a name has no word
// boundaries to split on.
//
// Everything else in this package finds a legal form by splitting a name into
// words. These scripts write no spaces, so "北京字节跳动科技有限公司" is one token
// and the gate had nothing to compare: a company did not match ITSELF, while
// Samsung and Hyundai met on the shared "주식회사". So the form is matched as a
// SUBSTRING anchored at each end. Unicode's own word segmentation does not help
// and says so — it breaks Han between every character and keeps a whole Hangul
// run as one word.
//
// EITHER END, because both are correct. The Korean registry's rule for deciding
// whether two names are the same removes the company-type string first and
// compares the remainder, which is this algorithm; Japanese company law requires
// "株式会社" to appear and says nothing about where. Japanese registration does
// treat the two orders as two registrable names, so this tier asks a human
// rather than merging.

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// cjkLegalForms are the company-type strings, matched against the ends of a name
// longest-first, in a loop. "有限公司" is a suffix of "有限责任公司", so a short
// match alone would leave "有限责任" on the brand — either the ordering or the
// repetition covers that. What needs BOTH is a form at each end.
//
// Simplified and Traditional are separate entries: NFKC does not bridge them, so
// a table with one spelling is blind to half the market.
var cjkLegalForms = []string{
	// Chinese, mainland and traditional.
	"有限责任公司", "有限責任公司",
	"股份有限公司",
	"个体工商户", "個體工商戶",
	"有限合伙", "有限夥",
	"合伙企业", "合夥企業",
	"有限公司",
	"股份公司",
	// Japanese.
	"一般社団法人", "一般財団法人",
	"株式会社", "株式會社",
	"有限会社", "有限會社",
	"合同会社", "合資会社", "合名会社",
	"医療法人",
	// Korean.
	"유한책임회사",
	"주식회사", "유한회사", "합자회사", "합명회사",
	"재단법인", "사단법인",
}

// cjkFormAliases map the squared and parenthesised spellings onto the form
// itself. NFKC does not: it rewrites ㈱ to "(株)" — the parentheses, not the
// words — so "㈱ガナ" would still share no substring with "株式会社ガナ".
var cjkFormAliases = map[string]string{
	"(株)": "株式会社",
	"(有)": "有限会社",
	"(资)": "合資会社",
	"(名)": "合名会社",
	"(주)": "주식회사",
	"(유)": "유한회사",
}

// cjkBrandTierWords LOOK like a legal form and are not: 集团 and 控股 name a
// different legal person from the company beside them, so the tables above must
// never contain them. Named here so a test can hold that intent.
var cjkBrandTierWords = []string{"集团", "集團", "控股", "控股集团"}

// cjkMaxFormsStripped bounds how many forms one name may shed. The strip is a
// loop over a shrinking string, so its cost is quadratic — measured, 10 000
// concatenated forms took 370ms — and it runs while the workspace-wide
// organization-name write lock is held. Four is past any real name.
const cjkMaxFormsStripped = 4

// cjkMinimumBrandRunes is the shortest name a strip may leave behind. A bare
// "株式会社" arrives in real exports and must not become the empty string, which
// would match every other empty string.
//
// One and not two: Japan and Korea set no floor on a brand's length, and dirty
// data sets none at all.
const cjkMinimumBrandRunes = 1

// carriesCJKForm answers whether this path owns the name: does it begin or end
// with one of the forms above?
//
// THE FORM, NOT THE SCRIPT. Claiming any name containing a single Han or Kana
// character is far too much — "CÔNG TY TNHH 東京 Việt" needs its Vietnamese
// handling and "Acme 漢 GmbH" its Latin suffix strip. A name that carries one of
// these forms is one of these markets whatever else it is written in.
func carriesCJKForm(name string) bool {
	for _, form := range cjkLegalForms {
		if strings.HasPrefix(name, form) || strings.HasSuffix(name, form) {
			return true
		}
	}
	return false
}

// normalizeCJKName folds a name to the form the tables are written in. NFKC
// rather than the NFD pass used elsewhere: it maps the full-width Latin and
// half-width katakana a Japanese registry emits onto their ordinary spellings.
//
// Variation selectors are removed by hand because normalization keeps them; they
// choose which shape of 葛 to draw, and two records of one company should not
// differ by one.
func normalizeCJKName(s string) string {
	folded := norm.NFKC.String(s)
	for alias, form := range cjkFormAliases {
		folded = strings.ReplaceAll(folded, alias, form)
	}
	var out strings.Builder
	out.Grow(len(folded))
	for _, r := range folded {
		if isVariationSelector(r) {
			continue
		}
		out.WriteRune(r)
	}
	// The SAME fold the rest of the package uses, not strings.ToLower: full
	// Unicode folding is what makes Greek final sigma and ß behave, and a second
	// spelling of "lowercase" here would make this path disagree with every
	// other one about the same Latin name.
	return normalizeName(strings.Join(strings.Fields(out.String()), " "))
}

// isVariationSelector reports the two ranges that pick a glyph shape.
func isVariationSelector(r rune) bool {
	return (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0xE0100 && r <= 0xE01EF)
}

// cjkNameForMatching is the name with its legal form removed from either end.
//
// Returns false for a name carrying none of these forms, so the caller can hand
// it to the word-level path. A name that DOES carry one belongs here whatever
// else it is written in: the word-level path would see it as a single token and
// then strip Latin forms out of it.
func cjkNameForMatching(s string) (string, bool) {
	// Normalized BEFORE the script test, because a squared form carries no Han
	// of its own: "㈱" is one compatibility character, and the script only
	// appears once NFKC and the alias map have rewritten it as "株式会社".
	name := normalizeCJKName(s)
	if !carriesCJKForm(name) {
		return "", false
	}
	for stripped := 0; stripped < cjkMaxFormsStripped; stripped++ {
		longest := ""
		for _, form := range cjkLegalForms {
			if len(form) <= len(longest) {
				continue
			}
			if strings.HasPrefix(name, form) || strings.HasSuffix(name, form) {
				longest = form
			}
		}
		if longest == "" {
			return name, true
		}
		trimmed := strings.TrimPrefix(name, longest)
		if trimmed == name {
			trimmed = strings.TrimSuffix(name, longest)
		}
		// A name that IS its legal form keeps it: an empty string would match
		// every other empty string, and a bare "株式会社" in an export names no
		// company at all.
		trimmed = strings.TrimSpace(trimmed)
		if len([]rune(trimmed)) < cjkMinimumBrandRunes {
			return name, true
		}
		name = trimmed
	}
	return name, true
}
