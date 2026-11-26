// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"regexp"
	"strings"
	"unicode"

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

	ruleViToneOpen        = "vi.tone-open"
	ruleViToneClosed      = "vi.tone-closed"
	ruleViDash            = "vi.dash"
	ruleViExclamation     = "vi.exclamation"
	ruleViNFC             = "vi.nfc"
	ruleViQuotes          = "vi.quotes"
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
// and, for a rule that readsEnglish, the English it was translated from.
type styleRule struct {
	id           string
	lang         textlang.Lang
	advice       string
	readsEnglish bool
	breaks       func(src sources, text, english string) bool
}

const besideEnglish, alone = true, false

var (
	braced = regexp.MustCompile(`\{[^}]*\}`)
	// formal and siblings port the census's pinned formal pattern, whose
	// (?! em) Go cannot read. particle is held by the page alone.
	formal   = wholeWord("quý vị|anh/chị|anh chị|kính thưa")
	siblings = wholeWord("anh chị em")
	particle = wholeWord("nhé|nha|ạ")
)

// prose drops the placeholders, which name a parameter and not copy. sep is
// what stands in their place: English joins the words around them, as the
// catalog gate does, and Vietnamese keeps them apart.
func prose(text, sep string) string {
	return braced.ReplaceAllString(verbs.ReplaceAllString(text, sep), sep)
}

// textOf prepares a sentence for one pattern.
type textOf func(src sources, text string) string

func enProse(_ sources, text string) string   { return prose(text, "") }
func viProse(_ sources, text string) string   { return prose(text, " ") }
func padded(_ sources, text string) string    { return " " + text + " " }
func asWritten(_ sources, text string) string { return text }

func matches(pattern string, prepare textOf) func(sources, string, string) bool {
	return func(src sources, text, _ string) bool { return src.patterns[pattern].MatchString(prepare(src, text)) }
}

func matchesHere(pattern *regexp.Regexp, prepare textOf) func(sources, string, string) bool {
	return func(src sources, text, _ string) bool { return pattern.MatchString(prepare(src, text)) }
}

func without(pattern string, prepare textOf) textOf {
	return func(src sources, text string) string { return erase(src.patterns[pattern], prepare(src, text)) }
}

// erase blanks every match. A match takes the character on each side, which an
// adjacent match needs as its own, so it repeats until nothing matches.
func erase(pattern *regexp.Regexp, text string) string {
	for {
		next := pattern.ReplaceAllString(text, " ")
		if next == text {
			return text
		}
		text = next
	}
}

var styleRules = []styleRule{
	// ui-copy-style.md fails the marks the Vietnamese census fails, wider than
	// the English catalog gate's em and en dash.
	{
		ruleEnDash, textlang.English, "uses a dash as punctuation; rewrite with a period, colon, comma or parentheses", alone,
		matches(patternDash, padded),
	},
	{
		ruleEnContraction, textlang.English, "uses a contraction; write it out (cannot, is not)", alone,
		matches(ruleEnContraction, enProse),
	},
	{
		ruleEnWe, textlang.English, "speaks as we; Margince is software, so name the actor or drop the pronoun", alone,
		matches(ruleEnWe, enProse),
	},
	{ruleEnI, textlang.English, "speaks as I; only an agent in a chat bubble says I", alone, matches(ruleEnI, enProse)},
	{
		ruleEnSpelling, textlang.English, "uses a British spelling; write American English", alone,
		matches(ruleEnSpelling, enProse),
	},
	{
		ruleEnAmpersand, textlang.English, "uses &; write and", alone,
		matches(ruleEnAmpersand, without(patternEnQuoted, asWritten)),
	},

	{
		ruleViToneOpen, textlang.Vietnamese, "marks an open oa, oe or uy on the second vowel; write hòa, xóa, tùy", alone,
		matches(ruleViToneOpen, viProse),
	},
	{
		ruleViToneClosed, textlang.Vietnamese, "marks the first vowel before a final consonant; write hoàn, khuyên", alone,
		matches(ruleViToneClosed, viProse),
	},
	{
		ruleViDash, textlang.Vietnamese, "uses a dash as punctuation; rewrite with a comma, period, colon or parentheses", alone,
		matches(patternDash, padded),
	},
	{
		ruleViExclamation, textlang.Vietnamese, "uses !; state the fact without it", alone,
		matches(ruleViExclamation, asWritten),
	},
	{
		ruleViNFC, textlang.Vietnamese, "holds a combining mark, so it is not NFC; write each letter precomposed", alone,
		func(_ sources, text, _ string) bool { return strings.ContainsFunc(text, isCombiningMark) },
	},
	{
		ruleViQuotes, textlang.Vietnamese, `quotes with a straight " or ', or with „, » or «; write “…” and ’`, alone,
		matches(ruleViQuotes, asWritten),
	},
	{
		ruleViToi, textlang.Vietnamese, "speaks as tôi; the product never speaks as itself", besideEnglish,
		func(src sources, text, english string) bool {
			return matches(ruleViToi, without(patternToiWe, viProse))(src, text, english) &&
				!src.patterns[patternSaysMe].MatchString(english)
		},
	},
	{
		ruleViChungToi, textlang.Vietnamese, "speaks as chúng tôi; only the data controller does", alone,
		matches(ruleViChungToi, viProse),
	},
	{
		ruleViChungTa, textlang.Vietnamese, "speaks as chúng ta or mình", alone,
		matches(ruleViChungTa, without(patternReflexive, viProse)),
	},
	{
		ruleViFormal, textlang.Vietnamese, "addresses the reader as quý vị, anh/chị or kính thưa", alone,
		func(src sources, text, _ string) bool { return formal.MatchString(erase(siblings, viProse(src, text))) },
	},
	{ruleViParticle, textlang.Vietnamese, "ends on nhé, nha or ạ", alone, matchesHere(particle, viProse)},
	{
		ruleViMemberAddress, textlang.Vietnamese, "says quý khách or vui lòng to a member; those are for an outsider", alone,
		matches(ruleViMemberAddress, viProse),
	},
	{
		ruleViOutsiderAddress, textlang.Vietnamese, "says bạn to an outsider; write quý khách", alone,
		matches(ruleViOutsiderAddress, viProse),
	},
	{
		ruleViRetired, textlang.Vietnamese, "uses a word the Vocabulary table of ui-copy-style-vi.md retires", alone,
		func(src sources, text, _ string) bool {
			return src.retired.MatchString(erase(src.ordinary, withoutPaths(src, text)))
		},
	},
}

// isCombiningMark is a nonspacing mark, which NFC Vietnamese never holds:
// every Vietnamese letter has a precomposed form.
func isCombiningMark(r rune) bool { return unicode.Is(unicode.Mn, r) }

// withoutPaths keeps a quoted vendor label, a file name and a URL out of the
// vocabulary rule. Each keeps the spelling the reader must match or type.
func withoutPaths(src sources, text string) string {
	return without(patternFileName, without(patternWebAddress, without(patternViQuoted, viProse)))(src, text)
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
		if waived(t, rule, text, english) {
			continue
		}
		t.Errorf("%s %s %s (%s, %s): %q\nRewrite the sentence. A line added to %s by hand is an exception that review must accept.",
			lang, name, rule.advice, rule.id, r.name, text, baselinePath)
	}
}
