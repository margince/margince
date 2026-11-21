// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealseat is what "this contact sits on the deal's buying committee"
// means, for every module that reads a deal_stakeholder edge.
//
// Held by: TestEveryDealSeatCurrencyTestUsesTheOneDefinition
// (backend/gates/dealseatcurrency_test.go).
package dealseat

import (
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// kind is the relationship kind of a seat on a deal.
const kind = "deal_stakeholder"

// CurrentSQL is a live deal_stakeholder edge whose End day has not arrived.
// An End date in the future still counts until that day. An End date of today
// has already ended, the rule employment.IsCurrentSQL keeps for a job. Today is
// the installation's day on the database clock, so every reader in one
// transaction asks it of the same day. alias is the relationship table's
// compile-time alias, or "" when the statement does not alias it.
func CurrentSQL(alias string) string {
	return CurrentAtSQL(alias, "now()")
}

// CurrentAtSQL is CurrentSQL as of the instant the SQL expression at names,
// such as the moment an offer was accepted. The End day is compared with the
// installation's day of that instant.
func CurrentAtSQL(alias, at string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return "(" + prefix + "kind = '" + kind + "' AND " + prefix + "archived_at IS NULL AND (" +
		prefix + "ended_at IS NULL OR " + prefix + "ended_at > " + occurredday.TodaySQL(at) + "))"
}

// SeatSQL is a deal_stakeholder edge, live or ended. It is for a check that
// asks what kind of edge a row is, not whether it is on the committee.
func SeatSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return prefix + "kind = '" + kind + "'"
}

// RemovalGrants are the relationship grants that admit taking an edge of
// edgeKind off. A deal seat comes off with the grant that put it on. Every
// other kind needs delete.
//
// Held by: TestTheFrontendRemovalRuleMirrorsTheServers
// (backend/gates/frontendremovalgrants_test.go).
func RemovalGrants(edgeKind string) []principal.Action {
	if edgeKind == kind {
		return []principal.Action{principal.ActionDelete, principal.ActionCreate}
	}
	return []principal.Action{principal.ActionDelete}
}

// AnyRemovalGrant admits a seat that may remove some kind of edge. It is the
// deal seat's set, because every other kind's set is delete alone.
func AnyRemovalGrant() []principal.Action {
	return RemovalGrants(kind)
}
