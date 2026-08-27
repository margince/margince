// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"strings"
	"testing"
	"time"
)

// cjkCompany is one real-shaped CJK name and the brand it must reduce to.
type cjkCompany struct {
	name  string
	brand string
	group string
}

// The corpus: the three scripts, each written the ways a real CRM receives them
// — with the legal form in front, behind, abbreviated to its squared character,
// and left off entirely.
//
// EVERY SAME-COMPANY PAIR HERE SCORED 0.0000 before this path existed, because
// these scripts write no spaces and the whole name arrived as a single token
// with nothing to compare. Toyota did not match itself. Meanwhile Samsung and
// Hyundai matched each other at 0.8533 on the shared "주식회사".
var cjkCorpus = []cjkCompany{
	// Chinese. The region and the industry stay: both are legally part of the
	// name, and either can BE the brand — 北京银行 is the Bank of Beijing.
	{name: "北京字节跳动科技有限公司", brand: "北京字节跳动科技", group: "bytedance"},
	{name: "北京字节跳动科技", brand: "北京字节跳动科技", group: "bytedance"},
	{name: "北京百度网讯科技有限公司", brand: "北京百度网讯科技", group: "baidu"},
	{name: "上海贸易有限公司", brand: "上海贸易", group: "shtrade"},
	{name: "深圳市腾讯计算机系统有限公司", brand: "深圳市腾讯计算机系统", group: "tencent"},
	{name: "台湾积体电路制造股份有限公司", brand: "台湾积体电路制造", group: "tsmc"},

	// Japanese. 前株 and 後株 are one company written two ways, and the squared
	// ㈱ is a third.
	{name: "株式会社トヨタ自動車", brand: "トヨタ自動車", group: "toyota"},
	{name: "トヨタ自動車株式会社", brand: "トヨタ自動車", group: "toyota"},
	{name: "㈱トヨタ自動車", brand: "トヨタ自動車", group: "toyota"},
	{name: "トヨタ自動車", brand: "トヨタ自動車", group: "toyota"},
	{name: "株式会社ホンダ技研", brand: "ホンダ技研", group: "honda"},
	{name: "有限会社山田製作所", brand: "山田製作所", group: "yamada"},
	{name: "㍿ホンダ技研", brand: "ホンダ技研", group: "honda"},
	{name: "ＡＢＣ株式会社", brand: "abc", group: "abc"},
	{name: "ABC株式会社", brand: "abc", group: "abc"},

	// Korean. The registry's own rule removes the type string before comparing,
	// so a spaced and an unspaced spelling are one name.
	{name: "주식회사 삼성전자", brand: "삼성전자", group: "samsung"},
	{name: "삼성전자주식회사", brand: "삼성전자", group: "samsung"},
	{name: "삼성전자", brand: "삼성전자", group: "samsung"},
	{name: "주식회사 현대자동차", brand: "현대자동차", group: "hyundai"},
	{name: "(주)카카오", brand: "카카오", group: "kakao"},
	{name: "카카오", brand: "카카오", group: "kakao"},
}

// Every CJK name reduces to the company inside it.
func TestEveryCJKNameReducesToItsBrand(t *testing.T) {
	for _, company := range cjkCorpus {
		if got := orgNameForMatching(company.name); got != company.brand {
			t.Errorf("%q reduces to %q, want %q — the comparison will be made on "+
				"the wrong characters", company.name, got, company.brand)
		}
	}
}

// No two different CJK companies score as one. Exhaustive over every
// cross-company pair.
func TestNoCJKCompanyMatchesAnother(t *testing.T) {
	pairs := 0
	for i, left := range cjkCorpus {
		for _, right := range cjkCorpus[i+1:] {
			if left.group == right.group {
				continue
			}
			pairs++
			if got := bestOrgNamePairing(left.name, "", right.name, "").Confidence; got >= dedupeReviewThreshold {
				t.Errorf("%q vs %q scores %.4f, at or above the %.2f review "+
					"threshold — these are different companies",
					left.name, right.name, got, dedupeReviewThreshold)
			}
		}
	}
	if want := cjkCrossCompanyPairs(); pairs != want {
		t.Fatalf("compared %d pairs, expected %d — the census is running short",
			pairs, want)
	}
}

// THE MISSED DUPLICATES. Every pairing within a company must meet, and this is
// the half of the defect that was silent: a company that does not match itself
// produces no warning, no queue entry and nothing for anyone to notice.
func TestEveryCJKDuplicateStillMeets(t *testing.T) {
	for i, left := range cjkCorpus {
		for _, right := range cjkCorpus[i+1:] {
			if left.group != right.group {
				continue
			}
			if got := bestOrgNamePairing(left.name, "", right.name, "").Confidence; got < dedupeReviewThreshold {
				t.Errorf("%q vs %q scores %.4f, below the %.2f review threshold — "+
					"this is one company written two ways",
					left.name, right.name, got, dedupeReviewThreshold)
			}
		}
	}
}

// A PARENT IS NOT ITS OWN SUBSIDIARY.
//
// 集团 (group) and 控股 (holding) name a different legal person from the company
// beside them: 中国中车集团有限公司 is the state-owned parent and
// 中国中车股份有限公司 its listed subsidiary, and the parent holds 54% of it.
// Stripping the word would file the two as one company, and this is the standard
// shape of the largest Chinese groups rather than an edge case.
func TestAChineseGroupIsNotItsListedCompany(t *testing.T) {
	parent, subsidiary := "中国中车集团有限公司", "中国中车股份有限公司"
	if got := bestOrgNamePairing(parent, "", subsidiary, "").Confidence; got >= dedupeReviewThreshold {
		t.Errorf("%q vs %q scores %.4f — a group and its listed company are two "+
			"legal persons", parent, subsidiary, got)
	}
	// And the word survives the strip, which is what keeps them apart.
	for _, word := range cjkBrandTierWords {
		name := "中国中车" + word + "有限公司"
		if got := orgNameForMatching(name); !strings.Contains(got, word) {
			t.Errorf("%q reduces to %q, which has lost %q — that word names the "+
				"company, not its legal form", name, got, word)
		}
	}
}

// A NAME THAT IS ONLY ITS LEGAL FORM STILL NAMES SOMETHING.
//
// A bare "株式会社" or "㈜" arrives in real exports. Reducing it to the empty
// string would make it match every other empty string, so the strip stops.
func TestABareCJKFormIsNotEmptied(t *testing.T) {
	for _, bare := range []string{"株式会社", "㈱", "주식회사", "有限公司", "株式会社株式会社"} {
		if got := orgNameForMatching(bare); got == "" {
			t.Errorf("%q reduces to the empty string, which matches every other "+
				"empty name", bare)
		}
	}
}

// The table must be written the way a folded name is written, or an entry can
// never fire and no other test would notice.
func TestCJKFormsMatchTheirOwnNormalization(t *testing.T) {
	if len(cjkLegalForms) == 0 {
		t.Fatal("no CJK forms declared — the census would pass against nothing")
	}
	for _, form := range cjkLegalForms {
		if got := normalizeCJKName(form); got != form {
			t.Errorf("form %q normalizes to %q — written this way it can never "+
				"match a real name", form, got)
		}
	}
	// Longest-match is what keeps a shorter form from leaving the remains of a
	// longer one, so every form that CONTAINS another must be reachable.
	for _, long := range cjkLegalForms {
		for _, short := range cjkLegalForms {
			if long == short || !strings.HasSuffix(long, short) {
				continue
			}
			name := "ゼット" + long
			if got, _ := cjkNameForMatching(name); got != "ゼット" {
				t.Errorf("%q reduces to %q — the shorter form %q won over %q and "+
					"left part of a legal form behind", name, got, short, long)
			}
		}
	}
	// A form at EACH END needs the loop: stripping once leaves the other.
	for _, k := range []struct{ name, want string }{
		{"小米科技有限责任公司", "小米科技"},
		{"台湾积体电路制造股份有限公司", "台湾积体电路制造"},
		{"株式会社ガナ株式会社", "ガナ"},
		{"有限公司北京科技有限公司", "北京科技"},
	} {
		if got, _ := cjkNameForMatching(k.name); got != k.want {
			t.Errorf("%q reduces to %q, want %q — a shorter form matched first and "+
				"left the rest of a longer one behind", k.name, got, k.want)
		}
	}
}

// A LEGAL FORM INSIDE A BRAND IS PART OF THE BRAND.
//
// The strip matches only at the ends of a name, never in the middle. 公司宝 is a
// Chinese company whose name opens with the word for "company"; 会社四季報 is a
// Japanese periodical whose name opens with the word for "company". A rule that
// removed the form wherever it appeared would take the brand off both.
func TestACJKFormInsideABrandIsKept(t *testing.T) {
	for _, k := range []struct{ name, want string }{
		{"公司宝科技有限公司", "公司宝科技"},
		{"会社四季報株式会社", "会社四季報"},
		{"株式会社会社設立freee", "会社設立freee"},
	} {
		if got, _ := cjkNameForMatching(k.name); got != k.want {
			t.Errorf("%q reduces to %q, want %q — the form was taken from inside "+
				"the brand", k.name, got, k.want)
		}
	}
	// Every alias must resolve to a form the table actually carries.
	for alias, form := range cjkFormAliases {
		if !strings.Contains(normalizeCJKName("ゼット"+alias), form) {
			t.Errorf("alias %q does not resolve to %q", alias, form)
		}
	}
}

// THE STRIP IS BOUNDED, because the work happens while the workspace-wide
// organization-name write lock is held and `display_name` has no length cap in
// the contract. Measured before the bound, a name of 10 000 concatenated forms
// took 370ms; every other organization-name writer waits behind that.
func TestTheCJKStripIsBounded(t *testing.T) {
	name := strings.Repeat("株式会社", 20000) + "ガナ"
	start := time.Now()
	got, isCJK := cjkNameForMatching(name)
	elapsed := time.Since(start)
	if !isCJK {
		t.Fatal("a name of nothing but legal forms is still this path's to answer")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("stripping took %v — the bound is not holding, and this runs "+
			"under the organization-name write lock", elapsed)
	}
	// Bounded, not wrong: the brand is still found at the end of what remains.
	if !strings.HasSuffix(got, "ガナ") {
		t.Errorf("the bounded strip lost the brand: %q", got[max(0, len(got)-24):])
	}
}

// A name this path claims must actually CARRY one of its forms. A stray Han or
// Kana character in a Vietnamese, Latin or Russian name is not this path's
// business — routing it here left the other market's legal form in place.
func TestOnlyANameCarryingACJKFormIsClaimed(t *testing.T) {
	for _, k := range []struct{ name, want string }{
		{"CÔNG TY TNHH 東京 Việt", "東京 viet"},
		{"Acme 漢 GmbH", "acme 漢"},
		{"ООО Ромашка 中", "ромашка 中"},
	} {
		if _, isCJK := cjkNameForMatching(k.name); isCJK {
			t.Errorf("%q was claimed by the CJK path, which owns no form in it", k.name)
		}
		if got := orgNameForMatching(k.name); got != k.want {
			t.Errorf("%q reduces to %q, want %q — its own market's handling was "+
				"skipped", k.name, got, k.want)
		}
	}
	// And a name that DOES carry one is claimed, whatever else it is written in.
	for _, k := range []struct{ name, want string }{
		{"Toyota株式会社", "toyota"},
		{"ABC有限公司", "abc"},
		{"Toyota Motor 株式会社", "toyota motor"},
	} {
		if got := orgNameForMatching(k.name); got != k.want {
			t.Errorf("%q reduces to %q, want %q", k.name, got, k.want)
		}
	}
}

// The CJK path must not reach a name written in another script.
func TestTheCJKPathLeavesOtherScriptsAlone(t *testing.T) {
	for _, name := range []string{
		"Acme Inc", "ООО Ромашка", "شركة الراجحي", "CÔNG TY CỔ PHẦN SỮA VIỆT NAM",
		"บริษัท เมืองไทยประกันชีวิต จำกัด", "SIA Rimi Latvia",
	} {
		if _, stripped := cjkNameForMatching(name); stripped {
			t.Errorf("%q was handled by the CJK path, which owns no script in it", name)
		}
	}
}

// cjkCrossCompanyPairs counts what the precision census owes, from the shape of
// the corpus rather than by walking it again.
func cjkCrossCompanyPairs() int {
	perCompany := map[string]int{}
	for _, c := range cjkCorpus {
		perCompany[c.group]++
	}
	pairs := len(cjkCorpus) * (len(cjkCorpus) - 1) / 2
	for _, n := range perCompany {
		pairs -= n * (n - 1) / 2
	}
	return pairs
}
