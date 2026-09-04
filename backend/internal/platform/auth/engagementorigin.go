// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Which activities count as the record having been touched.
//
// Beside audienceaggregate.go, and here for the same reason: several modules ask
// one question in hand-written SQL, no module may import a sibling, and a
// question spelled per module drifts one copy at a time.

// OriginIsEngagement filters an activity aliased by name down to the rows that
// mean somebody actually engaged, excluding the ones the system wrote itself.
//
// TWO origins are excluded today. system_remediation is work the product files
// about a record — a forecast-assurance review task — and system_notice is a
// message the installation owes somebody, such as the confirm-details link.
// Neither says a buyer did anything, and both would otherwise refresh the very
// recency reading that noticed the silence: the rule that spotted a dormant
// record would switch itself off by acting on it.
//
// The alias is a parameter because the callers name the activity table
// differently. It returns a leading " AND ", so a caller concatenates it into a
// WHERE clause and the surrounding arguments keep their positions.
//
// A FRAGMENT rather than a list of values because the readers are hand-written
// statements, and what this prevents is silent: a third system origin added to
// the vocabulary and missed by one reader makes that one reading count the
// product's own mail as engagement, with nothing failing anywhere.
//
// Held by: TestEveryRecencyReadingExcludesTheSystemOrigins (backend/gates/recencyorigins_test.go)
func OriginIsEngagement(alias string) string {
	return " AND " + alias + ".origin NOT IN ('system_remediation', 'system_notice') "
}
