// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The legal forms that come BEFORE the company name, per market.
//
// A TABLE PER MARKET because the forms differ in how safely they can be removed.
// "ООО" is Cyrillic and collides with nothing. "PT" is two Latin letters, and
// "PT Solutions Physical Therapy" is a company — removing it there leaves a name
// matching every firm whose name begins "Solutions". So is "AO World", and so is
// "AS Roma"; orgformtables_test.go holds the rest.

// orgFormMarker is one legal form as it appears after folding, split into the
// words a name is split into.
//
// `ambiguous` marks a form whose letters are also an ordinary name. Such a form
// is only removed when something else about the name agrees that this is the
// market it claims — see corroboratedMarket in orgnameforms.go. An unambiguous
// form needs no corroboration: no company is called "ООО Something" by accident.
type orgFormMarker struct {
	tokens    []string
	ambiguous bool
}

// marketForms are the prefix legal forms of one market.
//
// `fillers` is the trade vocabulary stacking between the form and the brand,
// removed only from a name that carried this market's legal form — the
// abbreviations are short enough to be words elsewhere.
//
// `continuations` appear only as the second half of a stacked form, arriving
// without their leading word: "TỔNG CÔNG TY CỔ PHẦN" leaves a bare "cổ phần".
type marketForms struct {
	name          string
	prefixes      []orgFormMarker
	continuations []orgFormMarker
	fillers       []orgFormMarker
	// closers are the market's own TRAILING forms, which corroborate a short
	// leading one: "PT Astra International Tbk" is Indonesian because it ends in
	// "Tbk", and no English name does. The strip itself is the existing
	// trailing-suffix pass in NormalizeOrgName; these say what that suffix
	// proves about the front of the name.
	closers []string
	// suffixes are trailing forms this market's own strip removes, for a script
	// that legalSuffixes cannot reach: that map is keyed on whitespace-split
	// words, and Thai writes none inside a name.
	suffixes []string
	// suffixRuns are trailing forms of SEVERAL words, which legalSuffixes also
	// cannot reach for the same reason — it decides one word at a time and can
	// never see that "sp z o o" is one form.
	suffixRuns [][]string
	// syllabic marks a market whose brands are built from a small pool of
	// shared syllables, where ONE word in common is not evidence of identity.
	// Vietnamese "Hòa Bình" and "Hòa Phát" share "hoa" and are the country's
	// largest construction firm and its largest steelmaker. English brands are
	// built from rare words instead, where one shared word is nearly the whole
	// of the evidence, so the majority rule must not reach them.
	syllabic bool
}

// prefixMarkets carries the markets whose legal form precedes the name, in the
// order matchingFormOf consults them.
//
// Hand-written and versioned, for the reason orgNameStopwords is. Seeded from
// the ISO 20275 Entity Legal Form list and extended with the everyday
// abbreviations it does not carry: it holds "công ty cổ phần" but not "CTCP",
// and no Russian abbreviations at all.
var prefixMarkets = []marketForms{
	vietnam,
	indonesia,
	russiaAndCIS,
	baltics,
	romania,
	turkeyAndPoland,
	arabic,
	hebrew,
	thailand,
}

// unambiguous builds markers from forms that collide with nothing.
func unambiguous(forms ...[]string) []orgFormMarker {
	markers := make([]orgFormMarker, 0, len(forms))
	for _, tokens := range forms {
		markers = append(markers, orgFormMarker{tokens: tokens})
	}
	return markers
}

// ambiguousFormWords are the words of every ambiguous form, which the gate must
// not count as evidence: "SIA Rimi Latvia" and "SIA Maxima Latvija" are two
// Latvian grocers. They cannot be REMOVED without corroboration, so they stay in
// the string the score compares.
//
// Derived from the tables rather than written out again.
var ambiguousFormWord = ambiguousFormWords()

func ambiguousFormWords() map[string]bool {
	words := map[string]bool{}
	for _, market := range prefixMarkets {
		for _, marker := range market.prefixes {
			if !marker.ambiguous {
				continue
			}
			for _, word := range marker.tokens {
				words[word] = true
			}
		}
	}
	return words
}

// ambiguous builds markers from forms whose letters are also ordinary names.
func ambiguous(forms ...[]string) []orgFormMarker {
	markers := make([]orgFormMarker, 0, len(forms))
	for _, tokens := range forms {
		markers = append(markers, orgFormMarker{tokens: tokens, ambiguous: true})
	}
	return markers
}

// Vietnam writes the form, then the trade, then the brand: "CÔNG TY CỔ PHẦN SỮA
// VIỆT NAM" is the joint-stock company Sữa Việt Nam. Both spellings of each form
// are here because companies write themselves both ways.
//
// The syllables recur because the language builds its forms out of them, and a
// reader must be able to check the table against the language.
//
//nolint:goconst // Vietnamese words, not repeated magic strings — see above.
var vietnam = marketForms{
	name:     "vietnam",
	syllabic: true,
	prefixes: unambiguous(
		[]string{"cong", "ty", "trach", "nhiem", "huu", "han", "mot", "thanh", "vien"},
		[]string{"cong", "ty", "trach", "nhiem", "huu", "han"},
		[]string{"cong", "ty", "tnhh", "hai", "thanh", "vien", "tro", "len"},
		[]string{"cong", "ty", "tnhh", "mot", "thanh", "vien"},
		[]string{"cong", "ty", "tnhh", "mtv"},
		[]string{"cong", "ty", "tnhh"},
		[]string{"cty", "tnhh", "mtv"},
		[]string{"cty", "tnhh"},
		[]string{"ngan", "hang", "thuong", "mai", "co", "phan"},
		[]string{"ngan", "hang", "tmcp"},
		[]string{"cong", "ty", "co", "phan"},
		[]string{"cong", "ty", "cp"},
		[]string{"cty", "co", "phan"},
		[]string{"cty", "cp"},
		[]string{"ctcp"},
		[]string{"cong", "ty", "hop", "danh"},
		[]string{"cong", "ty", "lien", "doanh"},
		[]string{"doanh", "nghiep", "tu", "nhan"},
		[]string{"dntn"},
		[]string{"tong", "cong", "ty"},
		[]string{"tap", "doan"},
		[]string{"hop", "tac", "xa"},
		[]string{"cong", "ty"},
		[]string{"cty"},
	),
	continuations: unambiguous(
		[]string{"co", "phan"},
		[]string{"trach", "nhiem", "huu", "han", "mot", "thanh", "vien"},
		[]string{"trach", "nhiem", "huu", "han"},
		[]string{"tnhh", "mot", "thanh", "vien"},
		[]string{"tnhh", "mtv"},
		[]string{"tnhh"},
	),
	fillers: unambiguous(
		[]string{"xuat", "nhap", "khau"},
		[]string{"thuong", "mai"},
		[]string{"dich", "vu"},
		[]string{"dau", "tu"},
		[]string{"phat", "trien"},
		[]string{"xay", "dung"},
		[]string{"san", "xuat"},
		[]string{"cong", "nghe"},
		[]string{"ky", "thuat"},
		[]string{"giai", "phap"},
		[]string{"quoc", "te"},
		[]string{"xnk"},
		[]string{"tm"},
		[]string{"dv"},
		[]string{"sx"},
		[]string{"dt"},
		[]string{"va"},
	),
}

// Indonesia puts "PT" in front and, for a listed company, "Tbk" behind: the
// brand sits between them. "PT" and "CV" are two Latin letters and both are real
// company names elsewhere, so both are ambiguous. The spelled-out forms are not.
var indonesia = marketForms{
	name:    "indonesia",
	closers: []string{"tbk"},
	prefixes: append(
		unambiguous(
			[]string{"perseroan", "terbatas"},
			[]string{"perusahaan", "perseroan"},
			[]string{"perusahaan", "umum"},
			[]string{"commanditaire", "vennootschap"},
			[]string{"pt", "pma"},
			[]string{"pt", "pmdn"},
			[]string{"perum"},
			[]string{"persero"},
		),
		ambiguous(
			[]string{"pt"},
			[]string{"cv"},
			[]string{"fa"},
		)...,
	),
}

// Russia and the CIS write the form, then the brand, usually in guillemets.
// Cyrillic collides with nothing. The Latin transliterations do: "AO World" is a
// British retailer, "TOO Group" reads as an English word, and "IP" is a common
// abbreviation, so those are ambiguous.
var russiaAndCIS = marketForms{
	name: "russia-cis",
	prefixes: append(
		unambiguous(
			[]string{"ооо"}, []string{"зао"}, []string{"оао"}, []string{"пао"},
			[]string{"ао"}, []string{"нко"}, []string{"гуп"}, []string{"муп"},
			[]string{"тов"}, []string{"прат"}, []string{"пат"}, []string{"фоп"},
			[]string{"таа"}, []string{"зат"}, []string{"аат"},
			[]string{"жшс"}, []string{"ақ"},
			[]string{"ooo"}, []string{"zao"}, []string{"oao"}, []string{"pao"},
			[]string{"tov"}, []string{"prat"}, []string{"tzov"},
		),
		ambiguous(
			[]string{"ao"}, []string{"too"}, []string{"ip"}, []string{"chp"},
		)...,
	),
}

// The Baltic forms sit at either end by statute — Latvia's commercial law says
// "at the beginning or end" — so they are stripped from the front here and by
// the existing trailing-suffix strip when they trail. Every one is short Latin
// and collides: SIA Engineering, AB InBev, AS Roma, MB Financial.
var baltics = marketForms{
	name: "baltics",
	prefixes: ambiguous(
		[]string{"uab"}, []string{"sia"}, []string{"ou"},
		[]string{"ab"}, []string{"as"}, []string{"mb"},
		[]string{"ik"}, []string{"ii"}, []string{"tub"},
	),
}

// Romania's "S.C." is not a legal form at all — the commercial register never
// carried it, and the ISO list does not list it. It is a courtesy prefix meaning
// "commercial company", it carries no information, and it goes unconditionally.
// The forms themselves trail the name and are handled by the suffix strip.
var romania = marketForms{
	name: "romania",
	// The Romanian form at the END is what proves the bare "SC" at the front:
	// "SC Dacia SRL" is a Romanian company, "SC Johnson" is not.
	closers: []string{"srl", "sa", "sca", "snc", "pfa"},
	// "S.C." dotted is unmistakable — no company writes its brand that way.
	// Bare "SC" is not: SC Johnson is a real company, and stripping it left
	// "johnson", which then met "Johnson Controls" at 0.89. So the undotted
	// spelling is ambiguous and needs the name to agree, exactly like "PT".
	prefixes: append(
		unambiguous([]string{"s", "c"}),
		ambiguous([]string{"sc"})...,
	),
}

// Turkey and Poland write short dotted forms. After folding the dots become
// separators, so "Sp. z o.o." arrives as four words.
var turkeyAndPoland = marketForms{
	name: "turkey-poland",
	prefixes: unambiguous(
		[]string{"sp", "z", "o", "o"},
		[]string{"spolka", "z", "ograniczona", "odpowiedzialnoscia"},
	),
	// "Sp. z o.o." TRAILS the name far more often than it leads it, and it
	// cannot go in legalSuffixes: that map is consulted one word at a time, and
	// this form is four words after the dots become separators. Removed here as
	// a run, longest first, so "Alfa Sp. z o.o." reduces to "alfa" rather than
	// sharing four trailing words with every other Polish company.
	suffixRuns: [][]string{
		{"sp", "z", "o", "o"},
		{"spolka", "z", "ograniczona", "odpowiedzialnoscia"},
		{"ltd", "sti"},
	},
}

// Arabic names open with شركة (sharikat, "company"), and Persian with شرکت.
// Neither collides with anything in another script.
var arabic = marketForms{
	name: "arabic",
	// Written as they FOLD, not as they are typed. Arabic vocalization and the
	// hamza carriers are removed by the normalization above, so "مؤسسة" arrives
	// as "موسسة" — an entry spelled the typed way would never fire, which the
	// census in orgnameforms_test.go fails the build over.
	prefixes: unambiguous(
		[]string{"شركة"}, []string{"شركه"}, []string{"موسسة"}, []string{"شرکت"},
	),
}

// Thailand BRACKETS the brand: "บริษัท <brand> จำกัด" is "company <brand>
// limited", and a public company adds "(มหาชน)". Both halves are needed — the
// leading word alone leaves "จำกัด" on every name, which is the shared word two
// unrelated insurers met on.
//
// The trailing half is a closer rather than a suffix in legalSuffixes, because
// that map is consulted per WORD after a whitespace split and Thai writes no
// spaces inside a word; here it is matched as the last word of the name.
var thailand = marketForms{
	name:     "thailand",
	closers:  []string{"จำกัด", "มหาชน"},
	prefixes: unambiguous([]string{"บริษัท"}, []string{"หจก"}, []string{"ห้างหุ้นส่วนจำกัด"}),
	suffixes: []string{"จำกัด", "มหาชน"},
}

// Hebrew's בע"מ trails the name, but חברת ("company of") leads it.
var hebrew = marketForms{
	name:     "hebrew",
	prefixes: unambiguous([]string{"חברת"}),
	// בע"מ is the trailing form on nearly every Israeli company, and the
	// gershayim inside it becomes a separator, so it arrives as two words.
	suffixRuns: [][]string{{"בע", "מ"}},
}
