// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package weeklyplan owns the week a rep intends to have: the commitments they
// wrote down on Monday, what they need help with, and what their lead answered.
//
// The counterpart to compose/weekly, and deliberately not part of it.
// weekly_review is the PAST — assembled by a job, frozen at write, edited by
// nobody. A plan is authored by a person, carries an audit row and an event for
// every change, is gated by its own RBAC object, and has a SECOND writer in the
// rep's lead. A record with those properties is a module, not a snapshot.
//
// The two meet once a week, in one direction only: when the weekly job closes a
// week it asks this module to settle the plan's open commitments, then counts
// the outcome into the review it is about to freeze. Nothing here reads a
// review, and the review never writes a plan.
//
// Tables owned: weekly_plan, weekly_plan_commitment
package weeklyplan
