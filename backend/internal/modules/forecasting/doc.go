// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package forecasting owns what a workspace expects to close, and the record of
// what it expected on the days before.
//
// Two things live here that look alike and are not. A READING is derived: ask
// for a period and it is computed from the deals as they stand right now. A
// SNAPSHOT is frozen: the same readings, plus the per-deal rows they were
// summed from, recorded at write time and never recomputed on read. The frozen half is what makes
// movement answerable — "the number moved, and here is which deals moved it" is
// a question about two snapshots, and it cannot be asked of a figure that is
// re-derived each time it is read.
//
// Freezing means freezing every input, and exchange rates are the one that
// bites. The rate sheet is mutable: a rate corrected or backfilled next week
// would silently re-convert last week's snapshot, and the movement report would
// then blame the difference on the business rather than on the correction. So a
// contribution stores its own base amount, rate and rate date, and every later
// read serves the stored integer.
//
// The current call is an assertion by a person, not a derivation: a manager
// saying what they believe will close. It supersedes rather than overwrites, so
// the chain of what was believed when survives, and it writes no deal row —
// calling a number is not editing the pipeline.
//
// Tables owned: forecast_call, forecast_snapshot, forecast_contribution
package forecasting
