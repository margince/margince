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

var holdingStatuses = [...]string{Held, Unsure, HeldByOwner}

// HoldingStatuses isolates repair-query arguments from the shared authorization rule.
func HoldingStatuses() []string { return slices.Clone(holdingStatuses[:]) }

// Holds identifies the settled decisions that withhold thread history.
func Holds(status string) bool {
	return slices.Contains(holdingStatuses[:], status)
}

// HoldsSQL shares the holding-status rule with set-based readers.
func HoldsSQL(column string) string {
	return column + " IN ('" + strings.Join(holdingStatuses[:], "', '") + "')"
}
