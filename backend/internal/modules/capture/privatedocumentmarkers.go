// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// File names and career headings can require a hold without a model.
//
// A CV describes one candidate's career, and a model can read the mail that
// carries it as an ordinary application or offer. A scanned invoice or cost
// sheet has no text to read, so a model sees only the note that covers it.

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// cvTitlesAnywhere name a CV wherever they stand, spaced out or not. A PDF can
// merge the title into the line of the name beside it.
var cvTitlesAnywhere = []string{"curriculum vitae", "lebenslauf"}

// cvTitles is a line that names the document a CV. A numbered line is not
// one: "1.2 CV" is an entry in a list of folders.
var cvTitles = []string{"cv", "resume", "résumé"}

// cvSections are the headings a CV is built from, by the part of a career
// each names. A heading counts once per group.
var cvSections = []wordList{
	{
		"work experience", "professional experience", "employment history", "work history", "career history",
		"experience", "berufserfahrung", "beruflicher werdegang", "berufliche erfahrung",
	},
	{"education", "academic background", "ausbildung", "studium", "schulbildung"},
	{"skills", "technical skills", "core competencies", "kenntnisse", "fähigkeiten", "it-kenntnisse"},
	{"languages", "sprachen", "sprachkenntnisse"},
	{"certifications", "certificates", "zertifikate", "zertifizierungen"},
	{
		"date of birth", "geburtsdatum", "nationality", "staatsangehörigkeit", "personal details",
		"persönliche daten",
	},
	{
		"summary", "professional summary", "profile", "personal profile", "about me", "profil", "kurzprofil",
		"über mich",
	},
}

// minCVSections is how many groups of headings make a CV. One alone is a
// proposal's "Experience" or a job advert's "Skills".
const minCVSections = 2

// maxHeadingRunes is the longest line read as a heading. A sentence that
// starts "Experience shows" is prose.
const maxHeadingRunes = 40

// DescribesACareer says whether text is a CV: a line that titles it one, or
// headings from two parts of a career. The verdict reads only the start of a
// document, where a CV seldom has more than two headings.
func DescribesACareer(text string) bool {
	compact := strings.ReplaceAll(strings.ToLower(text), " ", "")
	if slices.ContainsFunc(cvTitlesAnywhere, func(title string) bool {
		return strings.Contains(compact, strings.ReplaceAll(title, " ", ""))
	}) {
		return true
	}
	seen := map[int]bool{}
	for line := range strings.SplitSeq(text, "\n") {
		heading := headingOf(line)
		if heading == "" {
			continue
		}
		numbered := unicode.IsDigit([]rune(strings.TrimSpace(line))[0])
		if !numbered && slices.ContainsFunc(cvTitles, func(title string) bool { return sameHeading(heading, title) }) {
			return true
		}
		for group, names := range cvSections {
			if names.startsOne(heading) || slices.ContainsFunc(names, func(name string) bool {
				return sameHeading(heading, name)
			}) {
				seen[group] = true
			}
		}
		if len(seen) >= minCVSections {
			return true
		}
	}
	return false
}

// headingOf is a short line in lower case without its bullet, number or
// colon, or "" for a line too long to be a heading.
func headingOf(line string) string {
	trimmed := strings.TrimFunc(line, func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	if trimmed == "" || utf8.RuneCountInString(trimmed) > maxHeadingRunes {
		return ""
	}
	return strings.ToLower(trimmed)
}

// sameHeading compares without spaces, since a designed CV spaces its
// headings out ("E XP ER I ENC E") and its PDF keeps the gaps.
func sameHeading(heading, name string) bool {
	return strings.ReplaceAll(heading, " ", "") == strings.ReplaceAll(name, " ", "")
}

// wordList is a list of headings or stems a name or line is matched against.
type wordList []string

// startsOne says whether heading starts with one of the list's names as a
// whole word.
func (list wordList) startsOne(heading string) bool {
	for _, name := range list {
		rest, ok := strings.CutPrefix(heading, name)
		if ok && (rest == "" || !unicode.IsLetter([]rune(rest)[0])) {
			return true
		}
	}
	return false
}

// cvFilenameWords name a CV.
var cvFilenameWords = wordSet(`cv lebenslauf resume résumé`)

// costFilenameWords name a staff-cost invoice or a cost sheet.
var costFilenameWords = wordSet(`inv invoice invoices rechnung rechnungen cost costs costing payroll salary
	salaries gehalt gehälter lohn`)

var sensitiveFilenameWords = wordSet(`passport passports reisepass reisepässe personalausweis visa befund befunde covid`)

var sensitiveFilenameStems = wordList{"passport", "reisepass", "personalausweis", "befund", "covid"}

// sensitiveFilenamePrefixes name the same only at the start of a word:
// "contestresults" and "latestresults" contain "testresult".
var sensitiveFilenamePrefixes = wordList{"idcard", "testresult"}

var sensitiveFilenamePhrases = []string{"id card", "test result", "test results", "medical result", "medical results"}

// cvFilenameStems and costFilenameStems name the same inside a longer word,
// as in "salaryinvoice". Only stems no ordinary word contains are listed.
var (
	cvFilenameStems   = wordList{"lebenslauf", "curriculum"}
	costFilenameStems = wordList{"invoice", "rechnung", "payroll", "salar", "gehalt", "gehält"}
)

// NamedAsCV says whether a file's name calls it a CV. That holds whatever the
// text says: the start of a CV the verdict reads may show one heading or none.
func NamedAsCV(filename string) bool {
	return slices.ContainsFunc(filenameWords(filename), func(word string) bool {
		return cvFilenameWords[word] || cvFilenameStems.inside(word)
	})
}

// NamedAsPersonnel identifies files whose unread contents require a personnel hold.
func NamedAsPersonnel(filename string) bool {
	return slices.ContainsFunc(filenameWords(filename), func(word string) bool {
		return cvFilenameWords[word] || costFilenameWords[word] || cvFilenameStems.inside(word) || costFilenameStems.inside(word)
	})
}

// NamedAsPrivate says whether a file's name calls it an invoice, a cost sheet
// or a CV, identity document or medical result. For an unread file, its name
// is all there is to judge. An invoice it can read is judged by its text,
// because most invoices bill no one's pay.
func NamedAsPrivate(filename string) bool {
	words := filenameWords(filename)
	name := " " + strings.Join(words, " ") + " "
	return NamedAsPersonnel(filename) || slices.ContainsFunc(words, func(word string) bool {
		return sensitiveFilenameWords[word] || sensitiveFilenameStems.inside(word) ||
			slices.ContainsFunc(sensitiveFilenamePrefixes, func(prefix string) bool { return strings.HasPrefix(word, prefix) })
	}) || slices.ContainsFunc(sensitiveFilenamePhrases, func(phrase string) bool {
		return strings.Contains(name, " "+phrase+" ")
	})
}

// inside says whether word contains one of the list's stems.
func (list wordList) inside(word string) bool {
	return slices.ContainsFunc(list, func(stem string) bool { return strings.Contains(word, stem) })
}

// filenameWords cuts a file name at every non-letter and at each lower-case
// letter followed by an upper-case one, so "PhamHoangCV" yields "cv". It
// composes accents first and drops invisible characters, which a sender can
// put inside a word to break it.
func filenameWords(filename string) []string {
	var words []string
	var word []rune
	filename = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, norm.NFC.String(filename))
	flush := func() {
		if len(word) > 0 {
			words = append(words, strings.ToLower(string(word)))
			word = word[:0]
		}
	}
	for _, r := range filename {
		switch {
		case !unicode.IsLetter(r):
			flush()
		case unicode.IsUpper(r) && len(word) > 0 && unicode.IsLower(word[len(word)-1]):
			flush()
			word = append(word, r)
		default:
			word = append(word, r)
		}
	}
	flush()
	return words
}
