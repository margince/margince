// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealrole names the buying roles a contact can hold on a deal.
//
// One spelling, because three packages were carrying their own: the deal-stage
// rules, the at-risk sweep and now the account coverage read all ask whether a
// deal has a champion, and a literal typed a third time is a third thing to
// keep in step with the contract's enum.
//
// A role is RECORDED, never inferred. The contract says so where the field is
// declared — "never inferred from a job title" — and this package holds only
// the vocabulary, deliberately offering no function that guesses one.
package dealrole

const (
	// Champion carries the deal inside the account.
	Champion = "champion"
	// EconomicBuyer signs for it.
	EconomicBuyer = "economic_buyer"
	// Blocker can stop it.
	Blocker = "blocker"
	// Influencer shapes the decision without making it.
	Influencer = "influencer"
	// User lives with what is bought.
	User = "user"
)

// Critical are the roles whose ABSENCE is worth reporting, in the order a deal
// needs them.
//
// Not the whole vocabulary: a deal with no recorded influencer or user is
// ordinary, and a gap list that names every unfilled role names nothing. These
// two are the ones whose absence changes what a rep does next.
var Critical = []string{Champion, EconomicBuyer}

// Shown is the full vocabulary, in the order a committee reads.
var Shown = []string{Champion, EconomicBuyer, Influencer, Blocker, User}
