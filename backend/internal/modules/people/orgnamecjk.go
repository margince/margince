// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Org-name matching for Chinese, Japanese and Korean, where a name has no word
// boundaries to split on.
//
// WHY A SECOND PATH. Everything else in this package finds a legal form by
// splitting a name into words and comparing them. These scripts write no spaces:
// "北京字节跳动科技有限公司" is one token, so the gate had nothing to compare and
// answered no-match to every pair — including a company against ITSELF written
// without its legal form. Measured before this existed, all six of these scored
// 0.0000: 株式会社トヨタ自動車 against トヨタ自動車, 주식회사 삼성전자 against
// 삼성전자, 阿里巴巴集团控股有限公司 against 阿里巴巴. Missed duplicates, silently.
// Meanwhile 주식회사 삼성전자 and 주식회사 현대자동차 — Samsung and Hyundai —
// scored 0.8533 on the shared "주식회사".
//
// So the form is matched as a SUBSTRING anchored at each end, longest first.
// Unicode's own word segmentation (UAX #29) does not help: it breaks Han between
// every character and keeps a whole Hangul run as one word, and the standard
// says so itself. The alternative is a dictionary, which this package will not
// carry.
//
// A MATCH HERE IS A QUESTION, NOT A MERGE. Japanese registration treats
// 株式会社ガナ and ガナ株式会社 as two registrable names at one address, so the
// two really can be different companies. This tier answers "worth a human
// look", never "these are one record" — and against the alternative, which is
// the silence measured above, asking is the right error to make.
//
// THE KOREAN REGISTRY DOES EXACTLY THIS. Its rule for deciding whether two
// company names are the same removes the company-type string first and compares
// what is left, which is the algorithm below. It also settles a question this
// file would otherwise have to guess at: 가나주식회사 and 주식회사가나 are the
// same name, because once the type string is gone neither has a position left to
// differ in. Japan is the same — company law requires "株式会社" to appear in the
// name and says nothing about where.

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// cjkLegalForms are the company-type strings of the three scripts, matched
// against the ends of a name.
//
// Longest first, and the loop repeats. "有限公司" is a suffix of "有限责任公司"
// and of "股份有限公司", so a short match taken alone would leave "有限责任" on the
// brand — though either the ordering or the repetition covers that by itself.
// What needs BOTH is a name carrying a form at each end: strip once and
// "株式会社ガナ株式会社" keeps one of them.
//
// Simplified and Traditional are separate entries because Unicode does not
// bridge them — "责" and "責" are distinct characters and NFKC leaves both alone,
// so a table with only one spelling is blind to half the market.
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

// cjkFormAliases are the squared and parenthesised spellings of a legal form,
// mapped onto the form itself.
//
// NFKC DOES NOT DO THIS, which is the trap. It rewrites ㈱ to "(株)" and ㈜ to
// "(주)" — the parentheses, not the words — so a name written "㈱ガナ" still
// shares no substring with "株式会社ガナ" after normalization. Verified against
// the Go normalizer rather than assumed.
var cjkFormAliases = map[string]string{
	"(株)": "株式会社",
	"(有)": "有限会社",
	"(资)": "合資会社",
	"(名)": "合名会社",
	"(주)": "주식회사",
	"(유)": "유한회사",
}

// cjkBrandTierWords are words that LOOK like a legal form and are not.
//
// 集团 (group) and 控股 (holding) name a different legal person from the company
// they sit beside: 中国中车集团有限公司 is the state-owned parent and
// 中国中车股份有限公司 is its listed subsidiary, and the parent holds 54% of it.
// Stripping the word would file those two as one company — a parent merged with
// its own subsidiary, which is the opposite of deduplication, and this is the
// standard shape of the largest Chinese groups rather than an edge case.
//
// Listed here so the intent is written down and testable, not because the code
// needs a lookup: the forms above simply do not contain them.
var cjkBrandTierWords = []string{"集团", "集團", "控股", "控股集团"}

// cjkMaxFormsStripped bounds how many legal forms one name may shed.
//
// The strip is a loop over a shrinking string, so its cost is quadratic in the
// number of forms — measured, a name of 10 000 concatenated forms takes 370ms,
// and `display_name` is `text` with no maxLength in the contract. That work
// happens inside DedupeOrganizationForCreate, which holds the workspace-wide
// organization-name write lock, so an unbounded name pins every other
// organization-name writer behind it. The same reasoning as nameScoringMaxRunes
// (namesim.go), which bounds the metric for the same lock.
//
// FOUR is far past any real name: a company carries a form at one end, or at
// both, and "株式会社ガナ株式会社" is already the strange case. Past the bound the
// name keeps whatever forms are left, which for a name this shape is the same
// answer any comparison would give.
const cjkMaxFormsStripped = 4

// cjkMinimumBrandRunes is the shortest name a strip may leave behind.
//
// A name that is nothing but its legal form — a bare "株式会社" or "㈜" in a CRM
// export — must not become the empty string, which would match every other empty
// string. Mainland registration requires a brand of at least two characters, but
// Japan and Korea have no such floor and dirty data has no floor at all.
const cjkMinimumBrandRunes = 1

// carriesCJKForm answers whether this path owns the name: does it end or begin
// with one of the legal forms above?
//
// THE FORM, NOT THE SCRIPT. An earlier version claimed any name containing a
// single Han, Kana or Hangul character, which is far too much — "CÔNG TY TNHH
// 東京 Việt" is a Vietnamese company with a Han word in its name, and routing it
// here left its Vietnamese legal form in place and squashed its spaces. The same
// for "Acme 漢 GmbH", which needs the Latin suffix strip.
//
// A name that carries one of these forms is one of these markets, whatever else
// it is written in: "Toyota株式会社" and "ABC有限公司" are how those companies
// write themselves.
func carriesCJKForm(name string) bool {
	for _, form := range cjkLegalForms {
		if strings.HasPrefix(name, form) || strings.HasSuffix(name, form) {
			return true
		}
	}
	return false
}

// normalizeCJKName folds a name to the form the tables are written in.
//
// NFKC rather than the NFD pass the rest of this package uses: it is what maps
// the full-width Latin and half-width katakana a Japanese registry emits onto
// their ordinary spellings, and the ideographic space onto a plain one.
//
// Variation selectors are removed by hand because normalization keeps them.
// They are a rendering instruction — which shape of 葛 to draw — and two records
// of one company differ by them for no reason a reader could see.
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
// EITHER END, because both are correct and the difference is a style choice.
// Japanese writers put "株式会社" in front (前株) or behind (後株) with no legal
// distinction, and the Korean registry's own comparison rule removes the type
// string wherever it sits before deciding whether two names are the same.
//
// Repeated, because a name can carry a form at both ends, and a Chinese name can
// carry "集团有限公司" — where "有限公司" goes and "集团" stays.
//
// Returns false only when the name is written in some OTHER script, so the
// caller can hand it to the word-level path. A name that IS one of these scripts
// belongs here whether or not a form was found: the word-level path would split
// it into a single token and then strip Latin forms out of it, and "㈱" — which
// normalizes to a bare "株式会社" and has no brand to keep — came back empty from
// there.
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
