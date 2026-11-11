// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package threadverdict

import (
	"slices"
	"strings"
)

// Pending and the settled statuses share the ledger’s persisted spellings.
const (
	Pending       = "pending"
	Cleared       = "cleared"
	Held          = "held"
	Unsure        = "unsure"
	SharedByOwner = "shared_by_owner"
	HeldByOwner   = "held_by_owner"
)

var statusRules = map[string]struct{ holding, pending bool }{
	Pending:       {pending: true},
	Cleared:       {},
	SharedByOwner: {},
	Held:          {holding: true},
	Unsure:        {holding: true},
	HeldByOwner:   {holding: true},
}

// HoldingStatuses isolates repair-query arguments from the shared authorization rule.
func HoldingStatuses() []string {
	var statuses []string
	for status, rule := range statusRules {
		if rule.holding {
			statuses = append(statuses, status)
		}
	}
	slices.Sort(statuses)
	return statuses
}

// Holds identifies the settled decisions that withhold thread history.
func Holds(status string) bool {
	return statusRules[status].holding
}

// HoldsMessages includes a pending question while its mailbox asks for classification.
func HoldsMessages(status string) bool {
	rule := statusRules[status]
	return rule.holding || rule.pending
}

// HoldsSQL shares the holding-status rule with set-based readers.
func HoldsSQL(column string) string {
	return column + " IN ('" + strings.Join(HoldingStatuses(), "', '") + "')"
}
