// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// Who an admin may grant a mirror user-map to, in one place.
//
// Held by: TestEveryMappabilityStatementAsksTheHelper
// (backend/internal/modules/overlay/mappableseat_test.go), which reads the two
// SQL constants and fails if either carries a predicate beside the call, and
// TestTheMappableSeatPredicateNamesEveryHalf, which fails if this function
// drops one.
//
// It was spelled three times — listUserMapSQL (what the admin surface offers),
// selectUserMapTargetSQL (whether a target may be granted one) and
// usersMatchingEmail (what the automated sweep seeds) — each carrying a comment
// naming the other two as the thing that must move with it. Three statements
// held together by prose is one statement waiting to diverge, and the divergence
// this closes is what proved it: all three excluded an ARCHIVED seat and none
// excluded a DEACTIVATED one.
//
// That was not a deliberate difference. listUserMapSQL's own comment justified
// excluding an archived user because offering one "invites an admin to grant
// visibility to a seat that no longer logs in" — which is exactly as true of a
// deactivated seat, since deactivation sets `status` and leaves `archived_at`
// NULL. The predicate and its own stated reason disagreed.
// margince/margince#2592.
//
// BOTH HALVES, and neither implies the other — the same pair
// identity.LiveMemberSQL is, spelled here rather than imported because a module
// never imports a sibling (ADR-0054 §3) and identity owns app_user. That
// duplication is ratified by name in TestOnlyOneSpellingOfALiveMember, which is
// where the two spellings are held to agreeing.
//
// It governs GRANTING a mapping, not reading one. A mapping a deactivated seat
// already holds keeps working, and historical mail attributed to that seat keeps
// whatever mapping it had: nothing here revokes, and revocation is its own
// operation with its own audit row.
//
// alias is the table's alias in the caller's query, formatted in as an
// IDENTIFIER — so it must be a compile-time literal, never anything off a
// request. Every caller in this package passes "u".
func mappableSeatSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return "NOT " + prefix + "is_agent AND " + prefix + "status = 'active' AND " + prefix + "archived_at IS NULL"
}
