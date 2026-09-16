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
// arranged still come back in the order they arrived.
//
// AND THE ADDRESS ITSELF LAST, WHICH IS WHAT MAKES IT TOTAL. Nothing
// constrains the first three to be unique — a contact created with two
// addresses and no explicit positions has two rows at position 0, sharing the
// transaction's `now()` — and every reader appends LIMIT 1, so the planner
// picked whichever it reached first. Two surfaces asking the same question
// could disagree, which is the defect this constant exists to end rather than
// to relocate.
//
// `email` and not `id`, and the difference is not cosmetic. uq_contact_email_dedupe
// is UNIQUE on the address among LIVE rows, and every caller filters archived
// ones — so the address is guaranteed distinct across the candidate set, by an
// index rather than by luck. A uuid tie-break only looks total: v7 ids are
// monotonic where they are generated in sequence and not where they are not, so
// the same two rows sort one way on a laptop and the other way on CI. That was
// not a hypothetical; it is how this line got written.
//
// Held by: TestOneAnswerToWhichAddressAContactIsKnownBy
// (backend/gates/reachableaddress_test.go) — a statement that picks an address
// off contact_email and does not use this is a second answer, and fails there.
const ReachableOrder = ` ORDER BY is_primary DESC, position, created_at, email`
