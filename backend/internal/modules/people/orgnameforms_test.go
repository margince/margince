// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"strings"
	"testing"
)

// vietnameseCompany is one real-shaped name and the brand it must reduce to.
//
// `group` names the company behind the name. Two entries sharing a group are
// the same company written two ways and MUST meet at the fuzzy tier; two
// entries with different groups are different companies and must not. That one
// field is what lets every test below read the same corpus: precision asks
// about pairs across groups, recall about pairs within one.
type vietnameseCompany struct {
	name  string
	brand string
	group string
	// ambiguous marks a name that a HUMAN cannot separate from its neighbours
	// on the name alone. "Vàng Bạc Đá Quý Phú Nhuận" and "Vàng Bạc Phú Quý" are
	// two jewellers sharing four of six words; "Hòa Bình" and "An Bình" differ
	// in one syllable of two. The fuzzy tier's answer for these is to ask a
	// person, which is what it is for — so they are held out of the precision
	// census rather than being forced to a verdict the names do not support.
	//
	// Held out of the RECALL census too: an ambiguous pair may go either way,
	// and a test that demanded either answer would be asserting a coin toss.
	ambiguous bool
}

// The corpus: every legal form Vietnam uses, the trade vocabulary that stacks
// after it, and the spellings a real CRM receives — written out in full,
// abbreviated, and typed without diacritics by someone on a foreign keyboard.
//
// WHY IT IS THIS BIG. The names in this market differ in a smaller part of
// themselves than names anywhere else in the estate: two unrelated companies
// can share their first six words. A handful of hand-picked pairs would pass
// against a fix that is still wrong for the seventh form, so the precision test
// below asks EVERY pair rather than a chosen few, and this list is what makes
// that census worth running. A new form or a new filler belongs here, and every
// test gains the case at once.
var vietnameseCorpus = []vietnameseCompany{
	// Cổ phần — the joint-stock form, and the one in the bug report.
	{name: "CÔNG TY CỔ PHẦN SỮA VIỆT NAM", brand: "sua viet nam", group: "vinamilk"},
	{name: "CTCP Sữa Việt Nam", brand: "sua viet nam", group: "vinamilk"},
	{name: "Sữa Việt Nam", brand: "sua viet nam", group: "vinamilk"},
	{name: "CÔNG TY CỔ PHẦN TẬP ĐOÀN HÒA PHÁT", brand: "hoa phat", group: "hoaphat"},
	{name: "CÔNG TY CỔ PHẦN CÔNG NGHỆ FPT", brand: "fpt", group: "fpt"},
	{name: "CÔNG TY CỔ PHẦN CÔNG NGHỆ CMC", brand: "cmc", group: "cmc"},
	{name: "CÔNG TY CỔ PHẦN VÀNG BẠC ĐÁ QUÝ PHÚ NHUẬN", brand: "vang bac da quy phu nhuan", group: "pnj", ambiguous: true},

	// Cổ phần with a long filler run — the pairs that share six leading words.
	{name: "CÔNG TY CỔ PHẦN ĐẦU TƯ XÂY DỰNG TRUNG NAM", brand: "trung nam", group: "trungnam"},
	{name: "CÔNG TY CỔ PHẦN ĐẦU TƯ XÂY DỰNG RICONS", brand: "ricons", group: "ricons"},
	{name: "CÔNG TY CỔ PHẦN ĐẦU TƯ XÂY DỰNG COTECCONS", brand: "coteccons", group: "coteccons"},

	// đ-carrying names, and the same company typed without diacritics.
	{name: "CÔNG TY CỔ PHẦN ĐẦU TƯ NAM LONG", brand: "nam long", group: "namlong"},
	{name: "Cong ty co phan dau tu Nam Long", brand: "nam long", group: "namlong"},
	{name: "Nam Long", brand: "nam long", group: "namlong"},

	// TNHH — limited liability, written both legally and colloquially.
	{name: "CÔNG TY TNHH THƯƠNG MẠI DỊCH VỤ TÂN HIỆP PHÁT", brand: "tan hiep phat", group: "thp"},
	{name: "Cty TNHH TM DV Tân Hiệp Phát", brand: "tan hiep phat", group: "thp"},
	{name: "Tân Hiệp Phát", brand: "tan hiep phat", group: "thp"},
	{name: "CÔNG TY TRÁCH NHIỆM HỮU HẠN THƯƠNG MẠI VĨNH PHÁT", brand: "vinh phat", group: "vinhphat"},
	{name: "CÔNG TY TNHH KỸ THUẬT MINH ĐỨC", brand: "minh duc", group: "minhduc"},

	// TNHH một thành viên — the single-member company, three spellings.
	{name: "CÔNG TY TNHH MỘT THÀNH VIÊN HÒA BÌNH", brand: "hoa binh", group: "hoabinh", ambiguous: true},
	{name: "CÔNG TY TRÁCH NHIỆM HỮU HẠN MỘT THÀNH VIÊN HÒA BÌNH", brand: "hoa binh", group: "hoabinh", ambiguous: true},
	{name: "Hòa Bình", brand: "hoa binh", group: "hoabinh", ambiguous: true},
	{name: "CÔNG TY TNHH MTV XUẤT NHẬP KHẨU AN BÌNH", brand: "an binh", group: "anbinh", ambiguous: true},
	{name: "CÔNG TY TNHH MỘT THÀNH VIÊN XUẤT NHẬP KHẨU AN BÌNH", brand: "an binh", group: "anbinh", ambiguous: true},
	{name: "An Bình", brand: "an binh", group: "anbinh", ambiguous: true},

	// TNHH hai thành viên trở lên — the multi-member form.
	{name: "CÔNG TY TNHH HAI THÀNH VIÊN TRỞ LÊN MINH QUANG", brand: "minh quang", group: "minhquang"},

	// The remaining forms, one company each.
	{name: "CÔNG TY HỢP DANH KIỂM TOÁN VIỆT", brand: "kiem toan viet", group: "kiemtoanviet"},
	{name: "CÔNG TY LIÊN DOANH PHÚ MỸ HƯNG", brand: "phu my hung", group: "phumyhung"},
	{name: "DOANH NGHIỆP TƯ NHÂN VÀNG BẠC PHÚ QUÝ", brand: "vang bac phu quy", group: "phuquy", ambiguous: true},
	{name: "DNTN Vàng Bạc Phú Quý", brand: "vang bac phu quy", group: "phuquy", ambiguous: true},
	{name: "TỔNG CÔNG TY HÀNG KHÔNG VIỆT NAM", brand: "hang khong viet nam", group: "vietnamairlines"},
	{name: "TỔNG CÔNG TY CỔ PHẦN BIA RƯỢU SÀI GÒN", brand: "bia ruou sai gon", group: "sabeco"},
	{name: "TẬP ĐOÀN VINGROUP", brand: "vingroup", group: "vingroup"},
	{name: "TẬP ĐOÀN ĐIỆN LỰC VIỆT NAM", brand: "dien luc viet nam", group: "evn"},
	{name: "HỢP TÁC XÃ NÔNG NGHIỆP ĐẠI THÀNH", brand: "nong nghiep dai thanh", group: "daithanh"},
	{name: "NGÂN HÀNG THƯƠNG MẠI CỔ PHẦN Á CHÂU", brand: "a chau", group: "acb"},
	{name: "NGÂN HÀNG TMCP KỸ THƯƠNG VIỆT NAM", brand: "ky thuong viet nam", group: "techcombank"},

	// Brands built from words that are elsewhere boilerplate. Each of these is
	// a real naming pattern and each would lose its identity to a fix that
	// deleted tokens instead of matching phrases.
	{name: "CÔNG TY TNHH CỎ MAY", brand: "co may", group: "comay"},
	{name: "CÔNG TY TNHH PHAN MINH", brand: "phan minh", group: "phanminh"},
	{name: "CÔNG TY CỔ PHẦN VIỆT AN", brand: "viet an", group: "vietan"},
	{name: "CÔNG TY CỔ PHẦN PHÁT ĐẠT", brand: "phat dat", group: "phatdat"},
	{name: "CÔNG TY CỔ PHẦN THƯƠNG MẠI BÁCH HÓA XANH", brand: "bach hoa xanh", group: "bachhoaxanh"},

	// Names whose last syllable is "an", which is also an English article. Long
	// An is a province; both are ordinary company names here, and both lost
	// that syllable to the article list before it was made position-aware.
	{name: "CÔNG TY CỔ PHẦN LONG AN", brand: "long an", group: "longan"},
	{name: "CÔNG TY TNHH HÒA AN", brand: "hoa an", group: "hoaan"},
}

// crossCompanyPairCount is how many pairs the precision census owes: every
// unambiguous name against every other belonging to a different company.
//
// The census asserts against this rather than a written-down number, so adding
// a name to the corpus cannot silently leave the count behind and let the scan
// run short while still reporting PASS.
// Counted from the SHAPE of the corpus — how many names each company has —
// rather than by walking the pairs again, so it is an independent arithmetic
// check rather than a second copy of the loop it is holding.
func crossCompanyPairCount() int {
	perCompany := map[string]int{}
	total := 0
	for _, c := range vietnameseCorpus {
		if c.ambiguous {
			continue
		}
		perCompany[c.group]++
		total++
	}
	// Every pair of names, less the pairs that name one company twice.
	pairs := total * (total - 1) / 2
	for _, n := range perCompany {
		pairs -= n * (n - 1) / 2
	}
	return pairs
}

// Every name in the market reduces to the company inside it.
//
// The single assertion the rest of the file rests on: if a name does not reduce
// to its brand, every pair it takes part in is being judged on the wrong string.
func TestEveryVietnameseNameReducesToItsBrand(t *testing.T) {
	for _, company := range vietnameseCorpus {
		if got := orgNameForMatching(company.name); got != company.brand {
			t.Errorf("%q reduces to %q, want %q — the comparison will be made "+
				"on the wrong words", company.name, got, company.brand)
		}
	}
}

// THE REGRESSION, asked of every pair rather than a chosen few.
//
// Before this change a Vietnamese name carried its legal form into the score,
// and Jaro-Winkler boosts a shared prefix — so companies that share nothing but
// the words "công ty cổ phần" scored above the review threshold and an operator
// was told a company was already in the CRM when it was not.
//
// EXHAUSTIVE ON PURPOSE. Every cross-company pair in the corpus, which is what
// a hand-picked list cannot be: the pair that breaks next is the one nobody
// thought to write down. A census that can fail short has already failed.
func TestNoVietnameseCompanyMatchesAnyOther(t *testing.T) {
	pairs := 0
	for i, left := range vietnameseCorpus {
		for _, right := range vietnameseCorpus[i+1:] {
			if left.group == right.group || left.ambiguous || right.ambiguous {
				continue
			}
			pairs++
			got := bestOrgNamePairing(left.name, "", right.name, "").Confidence
			if got >= dedupeReviewThreshold {
				t.Errorf("%q vs %q scores %.4f, at or above the %.2f review "+
					"threshold — these are different companies",
					left.name, right.name, got, dedupeReviewThreshold)
			}
		}
	}
	// The census must not quietly shrink: a corpus that stopped producing pairs
	// would report PASS while testing air. Counted from the corpus rather than
	// written as a number, so it cannot drift out of date as names are added.
	if want := crossCompanyPairCount(); pairs != want {
		t.Fatalf("compared %d pairs, expected %d — the census is running short",
			pairs, want)
	}
}

// The fix must not cost recall. Every pairing inside a group is one company
// written two ways, and every one has to reach a human.
func TestEveryVietnameseDuplicateStillMeets(t *testing.T) {
	for i, left := range vietnameseCorpus {
		for _, right := range vietnameseCorpus[i+1:] {
			if left.group != right.group || left.ambiguous {
				continue
			}
			got := bestOrgNamePairing(left.name, "", right.name, "").Confidence
			if got < dedupeReviewThreshold {
				t.Errorf("%q vs %q scores %.4f, below the %.2f review threshold "+
					"— this is one company and the duplicate would be missed",
					left.name, right.name, got, dedupeReviewThreshold)
			}
		}
	}
}

// A BRAND IS NOT BOILERPLATE BECAUSE IT IS SPELLED LIKE IT.
//
// The folded legal form is made of syllables that are also brand syllables:
// "cổ" folds to "co" and so does "Cỏ" in "Cỏ May"; "phần" folds to "phan" and so
// does the surname "Phan". A fix that deleted those tokens wherever they
// appeared would strip the identity out of these companies and match them
// against anything — which is why the strip matches whole phrases at the front
// of a name and nothing else.
func TestABrandIsNotDeletedBecauseItLooksLikeBoilerplate(t *testing.T) {
	kept := []struct{ name, brand string }{
		{"Cỏ May", "co may"},
		{"CÔNG TY TNHH CỎ MAY", "co may"},
		{"Phan Minh", "phan minh"},
		{"CÔNG TY TNHH PHAN MINH", "phan minh"},
		{"Việt An", "viet an"},
		{"CÔNG TY CỔ PHẦN VIỆT AN", "viet an"},
		{"Phát Đạt", "phat dat"},
		{"Long An", "long an"},
		{"Hòa An", "hoa an"},
		{"Việt An", "viet an"},
		{"CÔNG TY CỔ PHẦN SỮA VIỆT NAM", "sua viet nam"},
		// The brand is what is LEFT, and trade words in front of it are not part
		// of it however the company writes them: "Đầu Tư Xanh" is the green
		// investment company, and the company is "Xanh". This is the same answer
		// the English side gives "Digital Solutions GmbH".
		{"Đầu Tư Xanh", "xanh"},
		{"Tập Đoàn Xanh", "xanh"},
	}
	for _, k := range kept {
		if got := orgNameForMatching(k.name); got != k.brand {
			t.Errorf("%q reduces to %q, want %q — the brand was taken for "+
				"boilerplate", k.name, got, k.brand)
		}
		// AND THE GATE MUST STILL SEE THE WHOLE BRAND. Reducing the name
		// correctly is not enough: the gate drops stopwords from what is left,
		// so a Vietnamese syllable listed there would take half the company with
		// it — "Cỏ May" becomes the single word "may", "Hòa Phát" becomes "hoa".
		// The name still looks right; the identity the gate compares does not.
		if got, want := distinctiveOrgTokens(k.name), strings.Fields(k.brand); len(got) != len(want) {
			t.Errorf("the gate reads %q as %v, want the whole brand %v — a word "+
				"of the company is being dropped as market vocabulary",
				k.name, got, want)
		}
	}
}

// A name of nothing but boilerplate has not said which company it is, and two
// of them have not said they are the same one.
//
// The gate compares what is LEFT of a name. When nothing is left, there is no
// evidence — and an empty string must not match another empty string, which is
// the same rule bestOrgNamePairing already applies to two blank legal names.
func TestAllBoilerplateNamesDoNotMatchEachOther(t *testing.T) {
	boilerplate := []string{
		"CÔNG TY CỔ PHẦN THƯƠNG MẠI DỊCH VỤ",
		"CÔNG TY TNHH THƯƠNG MẠI",
		"CÔNG TY TNHH MỘT THÀNH VIÊN",
		"Công ty Cổ phần",
	}
	for _, name := range boilerplate {
		if got := orgNameForMatching(name); got != "" {
			t.Errorf("%q reduces to %q, want empty — it names no company",
				name, got)
		}
	}
	for i, left := range boilerplate {
		for _, right := range boilerplate[i+1:] {
			// The GATE is asked its own question, not only the score. The score
			// refuses an empty normalized name before the gate is reached, so a
			// test that asked only for the score would report PASS however the
			// gate answered — and the gate is what decides this for every
			// caller that reaches it another way.
			if sharesADistinctiveWord(left, right) {
				t.Errorf("the gate admits %q vs %q — neither names a company, so "+
					"there is nothing for them to share", left, right)
			}
			got := bestOrgNamePairing(left, "", right, "").Confidence
			if got >= dedupeReviewThreshold {
				t.Errorf("%q vs %q scores %.4f — two names made only of legal "+
					"form and trade words are not one company", left, right, got)
			}
		}
	}
}

// An entry that is not written the way a folded name is written can never
// match, and nothing else in the suite would notice: the phrase simply never
// fires, the legal form stays in the name, and the false positives come back.
//
// Reads the real tables, never a copy — a census over a copied list is the
// shape that fails short and reports PASS.
func TestPhraseTableEntriesMatchTheirOwnNormalization(t *testing.T) {
	tables := map[string][][]string{
		"vietnameseLegalFormPrefixes": vietnameseLegalFormPrefixes,
		"vietnameseSectorFillers":     vietnameseSectorFillers,
	}
	for name, table := range tables {
		if len(table) == 0 {
			t.Fatalf("%s is empty — the census would pass against nothing", name)
		}
		for _, phrase := range table {
			if len(phrase) == 0 {
				t.Errorf("%s carries an empty phrase, which matches every name", name)
				continue
			}
			joined := strings.Join(phrase, " ")
			if got := NormalizeOrgName(foldDStroke(joined)); got != joined {
				t.Errorf("%s entry %q normalizes to %q — written this way it can "+
					"never match a real name", name, joined, got)
			}
		}
	}
}

// The Vietnamese path must be invisible to every other market.
//
// orgNameForMatching only differs from NormalizeOrgName when a Vietnamese
// phrase is present, so on the corpus this tree was measured against the two
// must agree exactly. If they ever diverge here, the strip has reached a name
// it was never meant to touch.
func TestWesternNamesAreUnchanged(t *testing.T) {
	western := []string{
		"Acme Inc", "Acme GmbH", "Siemens AG", "basecom GmbH & Co. KG",
		"Co", "AG Systems", "Capital.com", "giba.or.kr", "Digital Ocean",
		"PricewaterhouseCoopers", "Hewlett.Packard", "3M Company",
		"Fenwick Software Pty Ltd", "The Alta Group", "Media Markt",
		"Health Care", "Arvato Systems", "Bytesforce", "Salesforce",
	}
	for _, name := range western {
		if got, want := orgNameForMatching(name), NormalizeOrgName(name); got != want {
			t.Errorf("%q: matching normalization gives %q but the key gives %q — "+
				"the Vietnamese strip reached a name it does not own",
				name, got, want)
		}
	}
}
