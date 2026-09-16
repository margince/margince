// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package contactaddress is what "the address this contact is known by" means,
// in one place every module can reach.
//
// A contact carries a LIST of addresses — several, each with a position and an
// is_primary flag, some archived — so naming ONE is a decision, and the
// decision is the order. It sat in modules/contacts and answered for that
// module only. A module never imports a sibling, so activities and consent
// hand-spelled the question instead, and the hand-spellings disagreed:
// activities broke ties on the row id and consent left `position` out
// altogether. A contact who had arranged their own addresses got one on the
// preference centre and another on a reply, each looking right on its own
// screen.
//
// THE ARCHIVED FILTER IS NOT IN HERE, and that is deliberate. An archived
// address is one somebody retired, and mail to it either bounces or reaches
// somebody who asked us to stop — so it is the half a caller must never inherit
// silently from a helper it did not read. Every caller spells
// `archived_at IS NULL` in its own WHERE clause where the next reader can see
// it, and TestOneAnswerToWhichAddressAContactIsKnownBy has a second arm
// checking that they do.
//
// Tier 0 rather than storekit, for the reason the employment package beside it
// gives: storekit owns no domain, and which address a contact is reachable at
// is a domain rule. stdlib only, which the tier requires — the constant is a
// SQL fragment and needs nothing.
package contactaddress

// ReachableOrder is the order a contact's live addresses are listed in, and the
// order the one address they are reachable at is picked from.
//
// `is_primary DESC` first, because a primary address is the one somebody chose.
// Then `position`, which is the record's own arrangement — a contact who moved
// an address up meant it. Then `created_at`, so two addresses a caller never
// arranged still come back in a stable order rather than whatever the planner
// returns.
//
// Held by: TestOneAnswerToWhichAddressAContactIsKnownBy
// (backend/gates/reachableaddress_test.go) — a statement that picks an address
// off contact_email and orders it itself is a second answer, and fails there.
const ReachableOrder = ` ORDER BY is_primary DESC, position, created_at`
