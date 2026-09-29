// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package continuity records what happened when the installation practised
// losing its data.
//
// Two numbers are published about recovery: at most an hour of data lost, at
// most four hours to get back. Both were claims with nothing behind them. A
// restore that is never rehearsed is a procedure nobody has read under
// pressure, and a rehearsal nobody wrote down proves nothing afterwards —
// which is precisely when somebody asks, usually during a review and usually
// about a date that has already passed.
//
// So a drill leaves a row, and both published numbers are DERIVED from it
// rather than stored beside it: the recovery window is finished minus started,
// and the data-loss window is started minus the point restored to. A stored
// duration can disagree with the timestamps it came from, and a number that
// disagrees with its own evidence is worse than no number at all.
//
// A FAILED drill is evidence too. A ledger that only recorded its successes
// would be a record of nothing, and the failure is the reading somebody most
// needs before they rely on the procedure.
package continuity
