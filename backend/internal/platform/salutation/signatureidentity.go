// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/platform/namematch"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// Signer says how a signature block names the contact.
type Signer int

const (
	// SignedByNobody is a block that names somebody else or nobody.
	SignedByNobody Signer = iota
	// SignedByAddress is a block that prints the contact's address and not
	// their name. Whose signature it is rests on the name read off it.
	SignedByAddress
	// SignedByName is a block that prints the contact's name.
	SignedByName
)

// SignerOf reports how a block names the contact. Whole words match, so a short
// name cannot match inside somebody else's. Words inside an address name
// nobody. A block that prints another address besides theirs is another
// signer's. A line naming someone as "Name <address>" is a quote or header
// line, and signs nothing.
func SignerOf(block, fullName, email string) Signer {
	folded := quotedSender.ReplaceAllString(namematch.Fold(block), "")
	unaddressed := printedAddress.ReplaceAllString(folded, " ")
	if namesAll(wordsOf(unaddressed), namematch.Fold(fullName)) || lineNames(unaddressed, fullName) {
		return SignedByName
	}
	email = correspondence.Fold(email)
	if email != "" && strings.Contains(folded, email) && !holdsOtherAddress(folded, email) {
		return SignedByAddress
	}
	return SignedByNobody
}

// lineNames reports a block line of two to four words that spells the display
// name another way. "John Smith" spells "J. Smith".
func lineNames(block, fullName string) bool {
	header := headerWords(fullName)
	if len(header) < 2 || strings.Contains(fullName, "@") {
		return false
	}
	display := readName{given: header[:len(header)-1], family: header[len(header)-1:]}
	for line := range strings.Lines(block) {
		if words := readWords(line); len(words) >= 2 && len(words) <= 4 && display.namedBy(words, false) {
			return true
		}
	}
	return false
}

// namesAll asks for two tokens when the name has two. A surname alone is shared
// by relatives and colleagues, and the name gate writes from what matched.
func namesAll(words map[string]bool, fullName string) bool {
	var tokens, matched int
	for token := range strings.FieldsSeq(fullName) {
		token = strings.Trim(token, ".,;:()<>\"'")
		if token == "" {
			continue
		}
		tokens++
		if words[token] {
			matched++
		}
	}
	return matched > 0 && matched >= min(tokens, 2)
}

func wordsOf(folded string) map[string]bool {
	words := make(map[string]bool)
	for _, word := range strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		words[word] = true
	}
	return words
}

var quotedSender = regexp.MustCompile(`(?m)^.*<[^<>\s]+@[^<>\s]+>.*$`)

var printedAddress = regexp.MustCompile(`[\p{L}\p{N}._%+-]+@[\p{L}\p{N}-]+(?:\.[\p{L}\p{N}-]+)+`)

// PrintedAddresses lists the mail addresses text prints, each as a whole token,
// lowercased.
func PrintedAddresses(text string) []string {
	return printedAddress.FindAllString(strings.ToLower(text), -1)
}

func holdsOtherAddress(folded, email string) bool {
	for _, address := range printedAddress.FindAllString(folded, -1) {
		if address != email {
			return true
		}
	}
	return false
}

// NameIsContacts reports whether the given and family name read off a
// signature name this contact. Their display name or their address must spell
// the same contact, as namematch compares names.
func NameIsContacts(given, family, fullName, email string) bool {
	read := readName{given: readWords(given), family: readWords(family)}
	if len(read.given) == 0 || len(read.family) == 0 {
		return false
	}
	if !strings.Contains(fullName, "@") && read.namedBy(headerWords(fullName), false) {
		return true
	}
	local, _, _ := strings.Cut(correspondence.Fold(email), "@")
	tokens, ok := namematch.LocalPartTokens(correspondence.LocalPartBase(local))
	if !ok {
		return false
	}
	for i := range tokens {
		tokens[i] = nameKey(tokens[i])
	}
	return read.namedBy(tokens, true)
}

// readName is a read name as folded words.
type readName struct{ given, family []string }

// namedBy reports whether words spell this name. Two or more words need the
// family name and a given name. One word must be a given name, because a lone
// family name is shared by relatives and colleagues. An address may also run
// the name together ("jdoe", "johndoe").
func (r readName) namedBy(words []string, address bool) bool {
	family := r.family[len(r.family)-1]
	if len(words) == 1 {
		word := words[0]
		if slices.ContainsFunc(r.given, func(g string) bool { return sameGivenName(word, g) }) {
			return true
		}
		return address && slices.Contains(runTogether(r.given[0], strings.Join(r.family, "")), word)
	}
	for at, word := range words {
		if !namematch.FamiliesAgree(word, family) {
			continue
		}
		for other, candidate := range words {
			if other != at && slices.ContainsFunc(r.given, func(g string) bool {
				return namematch.GivenNamesAgree(candidate, g) || nicknameOf(candidate, g)
			}) {
				return true
			}
		}
	}
	return false
}

// sameGivenName is the strict test for a lone word: the name, a shortened form
// of it ("Alex" for "Alexander") or a known nickname.
func sameGivenName(word, given string) bool {
	return strings.HasPrefix(given, word) || strings.HasPrefix(word, given) || nicknameOf(word, given)
}

// runTogether lists the ways an address writes a whole name as one word.
func runTogether(given, family string) []string {
	initial := textcut.Runes(given, 1)
	return []string{family, given + family, initial + family, family + given, family + initial}
}

// nameKey folds a name word as namematch compares names, with umlauts written
// out, so "Müller", "Muller" and "mueller" meet.
func nameKey(word string) string {
	return namematch.Fold(namematch.Digraphs(word))
}

func readWords(name string) []string {
	return strings.Fields(strings.NewReplacer(".", " ", ",", " ").Replace(nameKey(name)))
}

// headerWords is the display name as folded words, honorifics dropped.
func headerWords(fullName string) []string {
	given, family := namematch.SplitName(namematch.Digraphs(fullName))
	if family == "" {
		return nil
	}
	return append(strings.Fields(given), family)
}

// nicknames maps a common short form to the given names it stands for, where
// the two share no prefix. A shared prefix ("Alex", "Kate") needs no entry.
var nicknames = nicknameIndex(map[string][]string{
	"robert": {"bob", "bobby", "rob"}, "william": {"bill", "billy", "will"},
	"richard": {"dick", "rick"}, "james": {"jim", "jimmy"}, "john": {"jack"},
	"edward": {"ted", "ned"}, "theodore": {"ted"}, "margaret": {"peggy", "meg"},
	"elizabeth": {"liz", "beth", "betty"}, "alexander": {"sasha"}, "alexandra": {"sasha"},
	"henry": {"hank"}, "sarah": {"sally"}, "mary": {"molly", "polly"},
	"charles": {"chuck"}, "anthony": {"tony"},
})

// nicknameIndex turns a given name's short forms into a lookup by short form.
func nicknameIndex(byGiven map[string][]string) map[string][]string {
	index := map[string][]string{}
	for given, shorts := range byGiven {
		for _, short := range shorts {
			index[short] = append(index[short], given)
		}
	}
	return index
}

func nicknameOf(a, b string) bool {
	return slices.Contains(nicknames[a], b) || slices.Contains(nicknames[b], a)
}
