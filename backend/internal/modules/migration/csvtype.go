// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// ObjectOfMappings is the type whose own columns a header names strictly most,
// else "". suggested holds the header → target mapping proposed for each type;
// a header two types both map says nothing about which one the file holds.
func ObjectOfMappings(suggested map[string]map[string]string) string {
	owners := map[string][]string{}
	for object, mapping := range suggested {
		for header := range mapping {
			owners[header] = append(owners[header], object)
		}
	}
	hits := map[string]int{}
	for _, objects := range owners {
		if len(objects) == 1 {
			hits[objects[0]]++
		}
	}
	best, most, tied := "", 0, false
	for object, n := range hits {
		switch {
		case n > most:
			best, most, tied = object, n, false
		case n == most:
			tied = true
		}
	}
	if tied {
		return ""
	}
	return best
}
