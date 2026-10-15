// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "strings"

// Colleagues who only share a given name are different people. A shared employer
// or mail domain may lift a pair over the review bar only when the names also
// agree on the family name and the given names are close.
const (
	familyNameAgreement = 0.9
	givenNameAgreement  = 0.78
	// A nickname or a respelling ("Lee", "Leigh") keeps the start of the name.
	givenNameSharedPrefix = 2
)

// namesCorroborate reports whether the two names may be scored together with an
// employer signal. Without it a pair is held to its name weight alone, which is
// below the review bar.
func namesCorroborate(a, b string) bool {
	givenA, familyA := splitPersonName(a)
	givenB, familyB := splitPersonName(b)
	if familyA == "" || familyB == "" {
		return false
	}
	if jaroWinkler(familyA, familyB) < familyNameAgreement {
		return false
	}
	return givenNamesAgree(givenA, givenB)
}

// givenNamesAgree treats a missing given name or an initial as compatible, and a
// shortened form ("Alex", "Alexander") as the same name.
func givenNamesAgree(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
		return true
	}
	if sharedPrefixRunes(a, b) >= givenNameSharedPrefix {
		return true
	}
	return jaroWinkler(a, b) >= givenNameAgreement
}

// splitPersonName returns the folded given part and family name. "Doe, Jon" is
// read as "Jon Doe".
func splitPersonName(full string) (given, family string) {
	name := normalizeName(full)
	if before, after, ok := strings.Cut(name, ","); ok {
		name = after + " " + before
	}
	fields := strings.Fields(strings.NewReplacer(".", " ").Replace(name))
	if len(fields) == 0 {
		return "", ""
	}
	return strings.Join(fields[:len(fields)-1], " "), fields[len(fields)-1]
}

func sharedPrefixRunes(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n
}
