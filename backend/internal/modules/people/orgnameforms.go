// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The Vietnamese half of org-name matching: a name whose legal form and trade
// vocabulary come FIRST, with the brand at the end.
//
// WHY THIS EXISTS. PO-PARAM-1 strips a legal form that TRAILS the name — "Acme
// Inc", "Acme GmbH" — because that is where English and German put it. Vietnam
// puts it in front and spells it in several words: "CÔNG TY CỔ PHẦN SỮA VIỆT
// NAM" is Vinamilk, and the first three words are "joint-stock company". Every
// Vietnamese company therefore begins with the same run of words, and a
// character metric reads that shared run as shared identity: Jaro-Winkler
// boosts a shared PREFIX specifically, so two unrelated Vietnamese companies
// scored above the review threshold on their boilerplate alone.
//
// A PHRASE, NEVER A WORD. The folded syllables of the legal form are also real
// brand syllables: "cổ" and "cô" and "có" all fold to "co", "phần" and "phân"
// to "phan", and "Cỏ May" is a rice company whose whole name folds to "co may".
// So a word of a legal form is only removed as part of the WHOLE phrase, at the
// front of the name. Deleting the token "phan" wherever it appears would take
// the brand off "Phan Minh", which is the failure this file is written to
// avoid.
//
// WHAT THIS IS NOT. Not a second normalizer for the org-name KEY:
// NormalizeOrgName (namesim.go) is unchanged, and still produces the exact and
// grouping keys, so no stored key moves. This one is consulted only where two
// names are COMPARED — the gate (orgnamegate.go) and the score (dedupeorg.go).
// The difference matters: "Công ty CP Đầu tư ABC" and "Công ty CP Thương mại
// ABC" must compare as one candidate pair and must NOT group under one key.

import "strings"

// vietnameseLegalFormPrefixes are the forms a Vietnamese company name opens
// with, written as they appear AFTER folding (lowercase, unaccented, đ→d).
//
// A HAND-WRITTEN, VERSIONED LIST, read the same way as orgNameStopwords and for
// the same reason: a list derived from the workspace's own names would make two
// names match in one workspace and not in another, and would put a query inside
// the transaction that holds the organization-name write lock.
//
// Both spellings of each form, because both are how companies write themselves:
// the legal "công ty trách nhiệm hữu hạn" and the everyday "công ty tnhh", and
// "cty" for "công ty". A duplicate pair in this market is very often one
// company written out in full against the same company abbreviated.
//
// Order does not matter here — matching takes the LONGEST phrase that fits, so
// "cong ty co phan" wins over "cong ty" on the same name regardless of position
// in this slice.
//
// The syllables recur because the language builds its legal forms out of them,
// and the point of writing the table this way is that a reader can check it
// against the language. Naming "cong" as a constant would hide what the entries
// say, so the repetition is the readable form here rather than a smell.
//
//nolint:goconst // Vietnamese words, not repeated magic strings — see above.
var vietnameseLegalFormPrefixes = [][]string{
	{"cong", "ty", "trach", "nhiem", "huu", "han", "mot", "thanh", "vien"},
	{"cong", "ty", "trach", "nhiem", "huu", "han"},
	{"cong", "ty", "tnhh", "hai", "thanh", "vien", "tro", "len"},
	{"cong", "ty", "tnhh", "mot", "thanh", "vien"},
	{"cong", "ty", "tnhh", "mtv"},
	{"cong", "ty", "tnhh"},
	{"cty", "tnhh", "mtv"},
	{"cty", "tnhh"},
	{"ngan", "hang", "thuong", "mai", "co", "phan"},
	{"ngan", "hang", "tmcp"},
	{"cong", "ty", "co", "phan"},
	{"cong", "ty", "cp"},
	{"cty", "co", "phan"},
	{"cty", "cp"},
	{"ctcp"},
	{"cong", "ty", "hop", "danh"},
	{"cong", "ty", "lien", "doanh"},
	{"doanh", "nghiep", "tu", "nhan"},
	{"dntn"},
	{"tong", "cong", "ty"},
	{"tap", "doan"},
	{"hop", "tac", "xa"},
	{"cong", "ty"},
	{"cty"},
}

// vietnameseLegalFormContinuations are the forms that appear only as the SECOND
// half of a stacked one, where they arrive stripped of their leading "công ty".
//
// "TỔNG CÔNG TY CỔ PHẦN BIA RƯỢU SÀI GÒN" is a corporation that is also a
// joint-stock company: once "tổng công ty" is taken off the front, what remains
// opens with a bare "cổ phần".
//
// SEPARATE FROM THE TABLE ABOVE, because these words are only boilerplate in
// that position. A company that simply calls itself "Cổ Phần Xanh" has those
// words as its name, and a strip that ran them from the front of any name would
// take it. So they are consulted only after a form has already been removed.
var vietnameseLegalFormContinuations = [][]string{
	{"co", "phan"},
	{"trach", "nhiem", "huu", "han", "mot", "thanh", "vien"},
	{"trach", "nhiem", "huu", "han"},
	{"tnhh", "mot", "thanh", "vien"},
	{"tnhh", "mtv"},
	{"tnhh"},
}

// vietnameseSectorFillers are the trade words that stack between the legal form
// and the brand: "CÔNG TY TNHH THƯƠNG MẠI DỊCH VỤ TÂN HIỆP PHÁT" is a trading
// and services company called Tân Hiệp Phát.
//
// They are the market's vocabulary, not an identity — unrelated companies share
// long identical runs of them, and the same run appears in different orders. So
// they are removed for the purposes of comparison, exactly as "solutions" and
// "systems" are dropped by orgNameStopwords on the English side.
//
// STRIPPED ONLY AT THE FRONT, after the legal form. A filler word that appears
// later is part of the brand — "SỮA VIỆT NAM" ends in a word that is elsewhere
// a filler, and it is the name of the company.
var vietnameseSectorFillers = [][]string{
	{"xuat", "nhap", "khau"},
	{"thuong", "mai"},
	{"dich", "vu"},
	{"dau", "tu"},
	{"phat", "trien"},
	{"xay", "dung"},
	{"san", "xuat"},
	{"cong", "nghe"},
	{"ky", "thuat"},
	{"giai", "phap"},
	{"quoc", "te"},
	{"xnk"},
	{"tm"},
	{"dv"},
	{"sx"},
	{"dt"},
	{"va"},
}

// foldDStroke maps đ to d.
//
// U+0111 LATIN SMALL LETTER D WITH STROKE has no canonical decomposition, so
// the NFD pass in normalizeName cannot separate a combining mark from it and
// the letter survives the fold intact: "ĐẦU TƯ" becomes "đau tu", not "dau tu".
// Postgres f_unaccent DOES map it, so without this the same name folds two ways
// — "dau tu" in the database and "đau tu" in Go — and no entry in the tables
// above could ever match a name typed with the letter Vietnamese uses for one
// of its most common trade words.
//
// Deliberately NOT inside normalizeName, which is pinned as the input to the
// similarity metric (PO-PARAM-JW-2) and shared with person and lead matching.
// This is an org-name concern and stays on the org-name path.
func foldDStroke(s string) string {
	if !strings.ContainsAny(s, "đĐ") {
		return s
	}
	return strings.NewReplacer("đ", "d", "Đ", "D").Replace(s)
}

// stripLeadingPhrases removes phrases from the FRONT of a name, longest first,
// for as long as one matches.
//
// Repeated, because the forms stack: "TỔNG CÔNG TY CỔ PHẦN …" carries two, and
// a name may open with a legal form followed by three trade words.
//
// Longest-match, because the short phrases are prefixes of the long ones.
// Taking "cong ty" off "cong ty co phan x" would leave "co phan x" and put the
// remains of a legal form into the comparison.
func stripLeadingPhrases(fields []string, table [][]string) []string {
	for {
		longest := 0
		for _, phrase := range table {
			if len(phrase) > longest && len(phrase) <= len(fields) &&
				matchesAt(fields, phrase) {
				longest = len(phrase)
			}
		}
		if longest == 0 {
			return fields
		}
		fields = fields[longest:]
	}
}

// matchesAt answers whether the name opens with exactly this phrase.
func matchesAt(fields, phrase []string) bool {
	for i, word := range phrase {
		if strings.Trim(fields[i], ".") != word {
			return false
		}
	}
	return true
}

// orgNameForMatching is the name reduced to what could identify a company: the
// key with a Vietnamese legal form and its trailing trade vocabulary removed.
//
// IT MAY RETURN EMPTY, and that is an answer rather than a failure. A name that
// is nothing but a legal form and trade words — "CÔNG TY CỔ PHẦN THƯƠNG MẠI
// DỊCH VỤ" — has said nothing about WHICH company it is, and two such names
// have said nothing about being the same one. Callers treat empty as no
// evidence, never as a wildcard.
//
// This is the one place it differs from NormalizeOrgName, which must never
// empty a name because it produces a stored key and an empty key would collide
// with every other empty key. A comparison has no such obligation: it is
// allowed to say "I cannot tell these apart on their names".
func orgNameForMatching(s string) string {
	name, _ := matchingFormOf(s)
	return name
}

// matchingFormOf is orgNameForMatching plus the fact the caller sometimes needs:
// whether this name declared itself Vietnamese by carrying a legal form.
//
// THE TRADE VOCABULARY IS ONLY STRIPPED FROM A NAME THAT DID. Its abbreviations
// are two letters — "tm", "dv", "dt", "va" — and two letters mean something else
// in every other market: "VA Software" and "DT Systems" are companies whose
// first word this table would otherwise eat, leaving them equal to "Software"
// and "Systems". A legal form is a strong enough signal to act on; a bare
// two-letter word at the front of a name is not.
func matchingFormOf(s string) (string, bool) {
	fields := strings.Fields(NormalizeOrgName(foldDStroke(s)))
	stripped := stripLeadingPhrases(fields, vietnameseLegalFormPrefixes)
	vietnamese := len(stripped) < len(fields)
	if !vietnamese {
		return strings.Join(stripped, " "), false
	}
	stripped = stripLeadingPhrases(stripped, vietnameseLegalFormContinuations)
	return strings.Join(stripLeadingPhrases(stripped, vietnameseSectorFillers), " "), true
}
