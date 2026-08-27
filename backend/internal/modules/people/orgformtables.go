// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The legal forms that come BEFORE the company name, for every market that
// writes them that way.
//
// WHY A TABLE PER MARKET AND NOT ONE LIST. The forms differ in how safely they
// can be removed, and the difference is what this file is for. "ООО" is Cyrillic
// and collides with nothing, so it can go wherever it appears at the front.
// "PT" is two Latin letters, and "PT Solutions Physical Therapy" is a company in
// Georgia — removing it there leaves a name that matches every other firm whose
// name begins "Solutions". So the ambiguous markers are held to a second signal
// before they are believed, and the unambiguous ones are not.
//
// Measured against real companies. These ten are all real, and a rule that
// stripped a two-letter marker on position alone damages every one of them:
// PT Solutions Physical Therapy, AO World, AS Roma, CV Sciences, SIA
// Engineering, II-VI Incorporated, TOO Group, AB InBev, SA Recycling, MB
// Financial.

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
// `fillers` is the trade vocabulary that stacks between the form and the brand,
// and only Vietnam has one here: it is the market whose names are built that
// way. A filler is only ever removed from a name that carried this market's
// legal form, because the abbreviations are short enough to be words elsewhere.
//
// `continuations` are forms that appear only as the second half of a stacked
// one, where they arrive without their leading word — "TỔNG CÔNG TY CỔ PHẦN"
// leaves a bare "cổ phần" once "tổng công ty" is gone. They are consulted only
// after a form has already been removed, so a name that simply begins with those
// words keeps them.
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
// A HAND-WRITTEN, VERSIONED LIST, read the same way as orgNameStopwords and for
// the same reason: a list derived from the workspace's own names would make two
// names match in one workspace and not in another, and would put a query inside
// the transaction that holds the organization-name write lock.
//
// Seeded from the ISO 20275 Entity Legal Form list, which is the registry of
// what each jurisdiction actually recognises, then extended with the everyday
// abbreviations that registry does not carry — it holds "công ty cổ phần" but
// not "CTCP", and no Russian abbreviations at all.
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
// not count as evidence of identity.
//
// These forms cannot be REMOVED from a name without corroboration — "PT
// Solutions" is a company and "AO World" is another — so they stay in the string
// the score compares. But a word that is a legal form somewhere is market
// vocabulary rather than a brand, and two names must not meet on it: "SIA Rimi
// Latvia" and "SIA Maxima Latvija" are two Latvian grocers, and "AS Roma" and
// "AS Monaco" are two football clubs.
//
// Derived from the tables rather than written out again, so a form added above
// cannot be forgotten here.
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
// are here because both are how companies write themselves — the legal "công ty
// trách nhiệm hữu hạn" and the everyday "công ty tnhh".
//
// The syllables recur because the language builds its legal forms out of them,
// and the point of writing the table this way is that a reader can check it
// against the language. Naming "cong" as a constant would hide what the entries
// say, so the repetition is the readable form here rather than a smell.
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
	name:     "romania",
	prefixes: unambiguous([]string{"s", "c"}, []string{"sc"}),
}

// Turkey and Poland write short dotted forms. After folding the dots become
// separators, so "Sp. z o.o." arrives as four words.
var turkeyAndPoland = marketForms{
	name: "turkey-poland",
	prefixes: unambiguous(
		[]string{"sp", "z", "o", "o"},
		[]string{"spolka", "z", "ograniczona", "odpowiedzialnoscia"},
	),
}

// Arabic names open with شركة (sharikat, "company"), and Persian with شرکت.
// Neither collides with anything in another script.
var arabic = marketForms{
	name: "arabic",
	prefixes: unambiguous(
		[]string{"شركة"}, []string{"شركه"}, []string{"مؤسسة"}, []string{"شرکت"},
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
}
