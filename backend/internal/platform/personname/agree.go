// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personname

import "strings"

// Colleagues who only share a given name are different people. Two names
// corroborate only when the family names agree and the given names are close.
const (
	familyNameAgreement = 0.9
	givenNameAgreement  = 0.78
	wholeNameAgreement  = 0.9
	// A nickname or a respelling ("Lee", "Leigh") keeps the start of the name.
	givenNameSharedPrefix = 2
)

// Corroborate reports whether two full names may name one person. Dedupe
// scores a shared employer only for a pair that corroborates.
func Corroborate(a, b string) bool {
	givenA, familyA := SplitName(a)
	givenB, familyB := SplitName(b)
	if familyA == "" || familyB == "" {
		return false
	}
	if FamiliesAgree(familyA, familyB) {
		return GivenNamesAgree(givenA, givenB)
	}
	// A suffix or a title kept by one source can push the family name out of
	// reach. The whole name may still read as one contact.
	return wholeNamesAgree(givenA, familyA, givenB, familyB)
}

// FamiliesAgree also compares the halves of a hyphenated family name, because
// "Doe-Smith" and "Doe" name one family where a register keeps only one half.
func FamiliesAgree(a, b string) bool {
	if JaroWinkler(a, b) >= familyNameAgreement {
		return true
	}
	for partA := range strings.SplitSeq(a, "-") {
		for partB := range strings.SplitSeq(b, "-") {
			if (partA != a || partB != b) && JaroWinkler(partA, partB) >= familyNameAgreement {
				return true
			}
		}
	}
	return false
}

func wholeNamesAgree(givenA, familyA, givenB, familyB string) bool {
	if givenA == "" || givenB == "" {
		return false
	}
	return JaroWinkler(givenA+" "+familyA, givenB+" "+familyB) >= wholeNameAgreement
}

// GivenNamesAgree treats a missing given name or an initial as compatible, and a
// shortened form ("Alex", "Alexander") as the same name.
func GivenNamesAgree(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
		return true
	}
	if sharedPrefixRunes(a, b) >= givenNameSharedPrefix {
		return true
	}
	return JaroWinkler(a, b) >= givenNameAgreement
}

// SplitName returns the folded given part and family name. "Doe, Jon" is
// read as "Jon Doe".
func SplitName(full string) (given, family string) {
	name := letterFold.Replace(Fold(full))
	if before, after, ok := strings.Cut(name, ","); ok {
		name = after + " " + before
	}
	var fields []string
	for field := range strings.FieldsSeq(strings.NewReplacer(".", " ", ",", " ").Replace(name)) {
		if !nameTitles[field] {
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return "", ""
	}
	return strings.Join(fields[:len(fields)-1], " "), fields[len(fields)-1]
}

// letterFold maps the letters that have no Unicode decomposition to the ASCII
// a register without them would write: "Đức" and "Duc" are one name.
var letterFold = strings.NewReplacer("đ", "d", "ł", "l", "ø", "o", "ħ", "h")

// nameTitles are honorifics and generational or academic suffixes, which a
// source may add or drop without naming a different contact.
var nameTitles = map[string]bool{
	"dr": true, "mr": true, "mrs": true, "ms": true, "prof": true,
	"jr": true, "sr": true, "ii": true, "iii": true, "phd": true, "md": true,
}

func sharedPrefixRunes(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n
}
