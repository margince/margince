// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The containers a PROVIDER defines, which mean the same thing in every
// mailbox.
//
// A container rule is otherwise personal, and rightly so: a label somebody
// made lives in one mailbox and names nothing in anybody else's, so binding
// every colleague's connection to it would match nothing forever. Gmail's four
// categories are not that. They are constants of the provider's own filing,
// identical in every Gmail account that exists, and an installation saying it
// captures no promotions is stating one fact about itself rather than reaching
// into somebody's label list.
//
// The same list is written into capture_exclusion_container_is_personal, and
// the two are held equal by a gate. A set that lives in a CHECK and in code is
// a set that drifts, and the drift is silent: the constraint would go on
// saying one thing while the seeder did another.

import "slices"

// Provider-defined container tokens, provider-qualified exactly as a rule
// stores them.
const (
	GmailCategoryPromotions = "gmail:CATEGORY_PROMOTIONS"
	GmailCategorySocial     = "gmail:CATEGORY_SOCIAL"
	GmailCategoryUpdates    = "gmail:CATEGORY_UPDATES"
	GmailCategoryForums     = "gmail:CATEGORY_FORUMS"
)

// providerDefinedContainers is the allow-list, in the order an installation
// meets them.
var providerDefinedContainers = []string{
	GmailCategoryPromotions,
	GmailCategorySocial,
	GmailCategoryUpdates,
	GmailCategoryForums,
}

// ProviderDefinedContainers returns the tokens a container rule may name at
// workspace scope. A copy, because a caller that sorted or appended to the
// shared slice would move what the constraint is held against.
func ProviderDefinedContainers() []string {
	return slices.Clone(providerDefinedContainers)
}

// IsProviderDefinedContainer reports whether a container value is one the
// provider defines rather than one somebody made.
func IsProviderDefinedContainer(value string) bool {
	return slices.Contains(providerDefinedContainers, value)
}
