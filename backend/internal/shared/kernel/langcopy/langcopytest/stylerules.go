// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"regexp"

	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Rule ids lead with the language, so a baseline entry names its language.
const (
	ruleEnDash        = "en.dash"
	ruleEnContraction = "en.contraction"
	ruleEnWe          = "en.we"
	ruleEnI           = "en.i"
	ruleEnSpelling    = "en.spelling"
	ruleEnAmpersand   = "en.ampersand"

	ruleViNFC             = "vi.nfc"
	ruleViToneOpen        = "vi.tone-open"
	ruleViToneClosed      = "vi.tone-closed"
	ruleViDash            = "vi.dash"
	ruleViExclamation     = "vi.exclamation"
	ruleViToi             = "vi.toi"
	ruleViChungToi        = "vi.chung-toi"
	ruleViChungTa         = "vi.chung-ta"
	ruleViFormal          = "vi.formal"
	ruleViParticle        = "vi.particle"
	ruleViMemberAddress   = "vi.member-address"
	ruleViOutsiderAddress = "vi.outsider-address"
	ruleViRetired         = "vi.retired"
)

// styleRule is one mechanical rule of a style page. breaks reads the sentence
// and, where a rule needs it, the English it was translated from.
type styleRule struct {
	id     string
	lang   textlang.Lang
	advice string
	breaks func(src sources, text, english string) bool
}

var (
	dash        = regexp.MustCompile(`[–—]|--|\s-\s`)
	bang        = regexp.MustCompile(`!`)
	ampersand   = regexp.MustCompile(`&`)
	curlyQuoted = regexp.MustCompile(`“[^“”]*”`)
	braced      = regexp.MustCompile(`\{[^}]*\}`)

	// Ported from copy-style-vi.census.ts. Go has no lookaround, so a
	// lookbehind becomes a consumed character or the start of the text.
	toneOpen = regexp.MustCompile(`(?i)(?:^|[^q])o[àáảãạ](?:$|[^\p{L}])|o[èéẻẽẹ](?:$|[^\p{L}])|` +
		`(?:^|[^q])u[ỳýỷỹỵ](?:$|[^\p{L}])`)
	toneClosed   = regexp.MustCompile(`(?i)[òóỏõọ][ae]\p{L}|(?:^|[^q])[ùúủũụ]y\p{L}`)
	webAddress   = regexp.MustCompile(`[a-z][a-z\d+.-]*://[^\s“”()]+`)
	fileName     = regexp.MustCompile(`[^\s“”(),]+\.[a-z][a-z\d]{0,4}(?:$|[^\p{L}\d])`)
	saysMe       = regexp.MustCompile(`(?i)\b(?:me|my|mine)\b`)
	toi          = wholeWord("tôi")
	chungToi     = wholeWord("chúng tôi")
	chungTa      = wholeWord("chúng ta|mình")
	reflexive    = wholeWord("(?:chính|tự|một) mình")
	formal       = wholeWord("quý vị|anh/chị|anh chị|kính thưa")
	siblings     = wholeWord("anh chị em")
	particle     = wholeWord("nhé|nha|ạ")
	outsiderWord = wholeWord("quý khách|vui lòng")
	memberWord   = wholeWord("bạn")
)

// prose drops the placeholders, which name a parameter and not copy. sep is
// what stands in their place: English joins the words around them, as the
// catalog gate does, and Vietnamese keeps them apart.
func prose(text, sep string) string {
	return braced.ReplaceAllString(verbs.ReplaceAllString(text, sep), sep)
}

func matches(pattern *regexp.Regexp, prepare func(string) string) func(sources, string, string) bool {
	return func(_ sources, text, _ string) bool { return pattern.MatchString(prepare(text)) }
}

func enProse(text string) string  { return prose(text, "") }
func viProse(text string) string  { return prose(text, " ") }
func padded(text string) string   { return " " + text + " " }
func verbatim(text string) string { return text }

func without(pattern *regexp.Regexp, prepare func(string) string) func(string) string {
	return func(text string) string { return pattern.ReplaceAllString(prepare(text), " ") }
}

func englishFrom(rule string) func(sources, string, string) bool {
	return func(src sources, text, _ string) bool { return src.english[rule].MatchString(enProse(text)) }
}

var styleRules = []styleRule{
	{ruleEnDash, textlang.English, "uses a dash as punctuation; rewrite with a period, colon, comma or parentheses",
		matches(dash, padded)},
	{ruleEnContraction, textlang.English, "uses a contraction; write it out (cannot, is not)",
		englishFrom(ruleEnContraction)},
	{ruleEnWe, textlang.English, "speaks as we; Margince is software, so name the actor or drop the pronoun",
		englishFrom(ruleEnWe)},
	{ruleEnI, textlang.English, "speaks as I; only an agent in a chat bubble says I", englishFrom(ruleEnI)},
	{ruleEnSpelling, textlang.English, "uses a British spelling; write American English", englishFrom(ruleEnSpelling)},
	{ruleEnAmpersand, textlang.English, "uses &; write and",
		matches(ampersand, without(curlyQuoted, verbatim))},

	{ruleViNFC, textlang.Vietnamese, "is not NFC; normalize it",
		func(_ sources, text, _ string) bool { return !norm.NFC.IsNormalString(text) }},
	{ruleViToneOpen, textlang.Vietnamese, "marks an open oa, oe or uy on the second vowel; write hòa, xóa, tùy",
		matches(toneOpen, viProse)},
	{ruleViToneClosed, textlang.Vietnamese, "marks the first vowel before a final consonant; write hoàn, khuyên",
		matches(toneClosed, viProse)},
	{ruleViDash, textlang.Vietnamese, "uses a dash as punctuation; rewrite with a comma, period, colon or parentheses",
		matches(dash, padded)},
	{ruleViExclamation, textlang.Vietnamese, "uses !; state the fact without it", matches(bang, verbatim)},
	{ruleViToi, textlang.Vietnamese, "speaks as tôi; the product never speaks as itself",
		func(_ sources, text, english string) bool {
			return toi.MatchString(without(chungToi, viProse)(text)) && !saysMe.MatchString(english)
		}},
	{ruleViChungToi, textlang.Vietnamese, "speaks as chúng tôi; only the data controller does",
		matches(chungToi, viProse)},
	{ruleViChungTa, textlang.Vietnamese, "speaks as chúng ta or mình", matches(chungTa, without(reflexive, viProse))},
	{ruleViFormal, textlang.Vietnamese, "addresses the reader as quý vị, anh/chị or kính thưa",
		matches(formal, without(siblings, viProse))},
	{ruleViParticle, textlang.Vietnamese, "ends on nhé, nha or ạ", matches(particle, viProse)},
	{ruleViMemberAddress, textlang.Vietnamese, "says quý khách or vui lòng to a member; those are for an outsider",
		matches(outsiderWord, viProse)},
	{ruleViOutsiderAddress, textlang.Vietnamese, "says bạn to an outsider; write quý khách",
		matches(memberWord, viProse)},
	{ruleViRetired, textlang.Vietnamese, "uses a word the Vocabulary table of ui-copy-style-vi.md retires",
		func(src sources, text, _ string) bool {
			return src.retired.MatchString(src.ordinary.ReplaceAllString(withoutPaths(text), " "))
		}},
}

// withoutPaths keeps a quoted vendor label, a file name and a URL out of the
// vocabulary rule. Each keeps the spelling the reader must match or type.
func withoutPaths(text string) string {
	unquoted := curlyQuoted.ReplaceAllString(viProse(text), " ")
	return fileName.ReplaceAllString(webAddress.ReplaceAllString(unquoted, " "), " ")
}

// ruleNamed answers the rule an id names, for a baseline entry.
func ruleNamed(id string) (styleRule, bool) {
	for _, rule := range styleRules {
		if rule.id == id {
			return rule, true
		}
	}
	return styleRule{}, false
}

// styleEntry holds one phrase to the English and Vietnamese pages. The English
// sentence comes along so "Giao cho tôi" may stand for "Assign to me".
func styleEntry(r Register) phraseCheck {
	return func(t reporter, name string, p langcopy.Phrase) {
		t.Helper()
		judge(t, r, textlang.English, name, p.En, p.En)
		judge(t, r, textlang.Vietnamese, name, p.Vi, p.En)
	}
}

// judge reports every rule of lang that a sentence breaks in this register,
// unless the style baseline already holds that finding.
func judge(t reporter, r Register, lang textlang.Lang, name, text, english string) {
	t.Helper()
	if text == "" {
		return
	}
	src, err := loadSources()
	if err != nil {
		t.Fatalf("the copy style rules cannot be read: %v", err)
	}
	for _, rule := range styleRules {
		if rule.lang != lang || !r.holds(rule.id) || !rule.breaks(src, text, english) {
			continue
		}
		if waived(t, rule.id, text) {
			continue
		}
		t.Errorf("%s %s %s (%s, %s): %q\nRewrite the sentence; %s waives only what was wrong before this check.",
			lang, name, rule.advice, rule.id, r.name, text, baselinePath)
	}
}
