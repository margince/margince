// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "testing"

// marketCompany is one real-shaped name from a prefix-form market, and the
// brand that identifies the company inside it.
//
// `group` names the company. Two entries sharing a group are one company written
// two ways and must meet at the fuzzy tier; two entries with different groups are
// different companies and must not.
type marketCompany struct {
	name  string
	brand string
	group string
	// sharesAGenericWord marks a name that meets a neighbour on ONE ordinary
	// word — a country ("Latvia" against "Latvija"), a sector ("Bank"). Those
	// pairs are held apart by nothing this file owns: one shared distinctive
	// word is enough evidence under the English rule, which predates the
	// prefix-form tables and is unchanged by them. They are here as real names
	// for the reduction and recall tests, and out of the precision census,
	// which would otherwise assert a fix that lives somewhere else.
	sharesAGenericWord bool
}

// Every market that writes its legal form in front of the name, with the
// spellings a real CRM receives: the local script, the Latin transliteration,
// and the bare brand somebody typed from memory.
//
// EVERY PAIR HERE SCORED ABOVE THE REVIEW THRESHOLD before these tables existed
// — 0.76 for two Russian companies, 0.87 for two Latvian ones, 0.91 for two
// Saudi ones. The names differ in a smaller part of themselves than Western
// names do, because the first word or two is the same in every name in the
// market, so a character metric reads the boilerplate as identity.
var marketCorpus = []marketCompany{
	// Indonesia. "PT" leads; a listed company also ends in "Tbk", so the brand
	// sits between them.
	{name: "PT Astra International Tbk", brand: "astra international", group: "astra"},
	{name: "Astra International", brand: "astra international", group: "astra"},
	{name: "PT Telkom Indonesia Tbk", brand: "telkom indonesia", group: "telkom"},
	{name: "PT Bank Mandiri Tbk", brand: "bank mandiri", group: "mandiri", sharesAGenericWord: true},
	{name: "PT Bank Central Asia Tbk", brand: "bank central asia", group: "bca", sharesAGenericWord: true},
	{name: "Perseroan Terbatas Gudang Garam", brand: "gudang garam", group: "gudanggaram"},

	// Russia and the CIS. Cyrillic and its Latin transliteration are the same
	// company, and the guillemets a registry writes are punctuation.
	{name: "ООО Ромашка", brand: "ромашка", group: "romashka"},
	{name: "ООО «Ромашка»", brand: "ромашка", group: "romashka"},
	{name: "ООО Василёк", brand: "василек", group: "vasilek"},
	{name: "ЗАО Спутник", brand: "спутник", group: "sputnik"},
	{name: "ПАО Газпром", brand: "газпром", group: "gazprom"},
	{name: "OOO Gazprom Neft", brand: "gazprom neft", group: "gazpromneft"},
	{name: "Gazprom Neft", brand: "gazprom neft", group: "gazpromneft"},
	{name: "OOO Lukoil", brand: "lukoil", group: "lukoil"},
	{name: "ТОВ Київстар", brand: "київстар", group: "kyivstar"},

	// The Baltics. Every form here is short Latin, so a bare "SIA Brand" cannot
	// be stripped — these prove the pairs stay apart anyway, on their brands.
	{name: "UAB Vilniaus Duona", brand: "uab vilniaus duona", group: "vilniausduona"},
	{name: "UAB Kauno Grudai", brand: "uab kauno grudai", group: "kaunogrudai"},
	{name: "SIA Rimi Latvia", brand: "sia rimi latvia", group: "rimi", sharesAGenericWord: true},
	{name: "SIA Maxima Latvija", brand: "sia maxima latvija", group: "maxima", sharesAGenericWord: true},

	// Romania. "S.C." was never part of a registered name and carries no
	// information at all, so it goes wherever it appears.
	{name: "S.C. Dacia S.R.L.", brand: "dacia", group: "dacia"},
	{name: "SC Dacia SRL", brand: "dacia", group: "dacia"},
	{name: "Dacia", brand: "dacia", group: "dacia"},
	{name: "S.C. Petrom S.A.", brand: "petrom", group: "petrom"},

	// Arabic. The script itself is the corroboration.
	{name: "شركة الراجحي", brand: "الراجحي", group: "rajhi"},
	{name: "شركة الأهلي", brand: "الاهلي", group: "ahli"},
	{name: "مؤسسة الفيصل", brand: "الفيصل", group: "faisal"},

	// Thailand. The vowels here are combining marks, and an unrestricted
	// accent-strip used to delete them — these names would have lost letters
	// before any comparison began.
	{name: "เมืองไทยประกันชีวิต", brand: "เมืองไทยประกันชีวิต", group: "muangthailife"},
	{name: "เมืองไทยประกันภัย", brand: "เมืองไทยประกันภัย", group: "muangthaiins"},
	{name: "กรุงไทย", brand: "กรุงไทย", group: "krungthai"},
	{name: "บริษัท เมืองไทยประกันชีวิต จำกัด", brand: "เมืองไทยประกันชีวิต", group: "muangthailife"},
	{name: "บริษัท เมืองไทยประกันชีวิต จำกัด (มหาชน)", brand: "เมืองไทยประกันชีวิต", group: "muangthailife"},
	{name: "บริษัท กรุงไทยการไฟฟ้า จำกัด", brand: "กรุงไทยการไฟฟ้า", group: "krungthaielec"},
	{name: "บริษัท เมืองไทยประกันภัย จำกัด", brand: "เมืองไทยประกันภัย", group: "muangthaiins"},
}

// No two different companies in any of these markets score as one.
//
// Exhaustive over every cross-company pair rather than a chosen few: the pair
// that breaks next is the one nobody thought to write down.
func TestNoCompanyInAPrefixMarketMatchesAnother(t *testing.T) {
	pairs := 0
	for i, left := range marketCorpus {
		for _, right := range marketCorpus[i+1:] {
			if left.group == right.group || left.sharesAGenericWord || right.sharesAGenericWord {
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
	if want := crossCompanyPairs(marketCorpus); pairs != want {
		t.Fatalf("compared %d pairs, expected %d — the census is running short",
			pairs, want)
	}
}

// One company written two ways still meets, which is what the strip is for.
func TestEveryPrefixMarketDuplicateStillMeets(t *testing.T) {
	for i, left := range marketCorpus {
		for _, right := range marketCorpus[i+1:] {
			if left.group != right.group {
				continue
			}
			if got := bestOrgNamePairing(left.name, "", right.name, "").Confidence; got < dedupeReviewThreshold {
				t.Errorf("%q vs %q scores %.4f, below the %.2f review threshold — "+
					"this is one company and the duplicate would be missed",
					left.name, right.name, got, dedupeReviewThreshold)
			}
		}
	}
}

// Every name reduces to the company inside it.
func TestEveryPrefixMarketNameReducesToItsBrand(t *testing.T) {
	for _, company := range marketCorpus {
		if got := orgNameForMatching(company.name); got != company.brand {
			t.Errorf("%q reduces to %q, want %q — the comparison will be made on "+
				"the wrong words", company.name, got, company.brand)
		}
	}
}

// A SHORT LATIN LEGAL FORM IS ALSO AN ORDINARY NAME, and every company here is
// real. Stripping "PT" from the front on position alone leaves "Solutions
// Physical Therapy", which then matches every firm in that sector — so an
// ambiguous form is only believed when the name itself agrees, and a plain
// English name never does.
func TestAnAmbiguousFormIsNotStrippedFromAWesternName(t *testing.T) {
	kept := []struct{ name, want string }{
		{"PT Solutions Physical Therapy", "pt solutions physical therapy"},
		{"AO World", "ao world"},
		{"AS Roma", "as roma"},
		{"CV Sciences", "cv sciences"},
		{"SIA Engineering", "sia engineering"},
		{"AB InBev", "ab inbev"},
		{"MB Financial", "mb financial"},
		{"TOO Group", "too group"},
		{"IP Group", "ip group"},
		{"FA Cup Ventures", "fa cup ventures"},
	}
	for _, k := range kept {
		if got := orgNameForMatching(k.name); got != k.want {
			t.Errorf("%q reduces to %q, want %q — another market's legal form was "+
				"read out of an ordinary English name", k.name, got, k.want)
		}
	}
}

// The corroboration that DOES let a short form go: the market's own bracketing
// suffix, or a name written in another script.
func TestAnAmbiguousFormGoesWhenTheNameAgrees(t *testing.T) {
	stripped := []struct{ name, want, why string }{
		{"PT Astra International Tbk", "astra international", "Tbk closes an Indonesian name"},
		{"PT Индустрия", "индустрия", "the rest of the name is not Latin"},
	}
	for _, s := range stripped {
		if got := orgNameForMatching(s.name); got != s.want {
			t.Errorf("%q reduces to %q, want %q — %s", s.name, got, s.want, s.why)
		}
	}
}

// A word that is another market's legal form is that market's vocabulary, and
// two names must not meet on it alone.
func TestAForeignLegalFormIsNotEvidenceOfIdentity(t *testing.T) {
	apart := [][2]string{
		{"AS Roma", "AS Monaco"},
		{"UAB Vilniaus Duona", "UAB Kauno Grudai"},
		{"PT Solutions Physical Therapy", "PT Astra International Tbk"},
	}
	for _, p := range apart {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got >= dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f — they share a legal form, not a company",
				p[0], p[1], got)
		}
	}
	// AND IT IS STILL EVIDENCE WHEN IT IS ALL THE NAME HAS. "PT Solutions"
	// reduces to "pt" once "solutions" is gone as English market vocabulary,
	// and it must still find its own longer name — this pair scores 0.88 on
	// main and a fix that silenced it would be trading a real duplicate away.
	meet := [][2]string{
		{"PT Solutions Physical Therapy", "PT Solutions"},
		{"AO World plc", "AO World"},
		{"AS Roma", "AS Roma SpA"},
		{"SIA Rimi Latvia", "Rimi Latvia"},
	}
	for _, p := range meet {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got < dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f, below the %.2f threshold — this is one "+
				"company", p[0], p[1], got, dedupeReviewThreshold)
		}
	}
}

// Thai vowels are combining marks, and stripping them as accents deleted real
// letters: two different insurers folded to nearly the same string.
func TestThaiNamesKeepTheirVowels(t *testing.T) {
	for _, name := range []string{"เมืองไทยประกันชีวิต", "กรุงไทย", "ลาวพัฒนา"} {
		if got := normalizeName(name); got != name {
			t.Errorf("%q folds to %q — Thai vowels are letters, not accents",
				name, got)
		}
	}
	// And the accent strip still does its job where the marks ARE accents.
	for _, k := range []struct{ in, want string }{
		{"Müller", "muller"},
		{"Bär", "bar"},
		{"Straße", "strasse"},
		{"Οδυσσεύς", "οδυσσευσ"},
		{"Василёк", "василек"},
	} {
		if got := normalizeName(k.in); got != k.want {
			t.Errorf("%q folds to %q, want %q — the accent strip must still run "+
				"on the alphabets it was built for", k.in, got, k.want)
		}
	}
}

// crossCompanyPairs counts the pairs a precision census owes, from the SHAPE of
// the corpus rather than by walking it again — an independent arithmetic check
// rather than a second copy of the loop it holds.
func crossCompanyPairs(corpus []marketCompany) int {
	perCompany := map[string]int{}
	total := 0
	for _, c := range corpus {
		if c.sharesAGenericWord {
			continue
		}
		perCompany[c.group]++
		total++
	}
	pairs := total * (total - 1) / 2
	for _, n := range perCompany {
		pairs -= n * (n - 1) / 2
	}
	return pairs
}
