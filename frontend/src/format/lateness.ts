// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Whether a promised moment is LATE, and by how many whole days.
//
// One place, because lateness was spelled once per screen and the spellings
// disagreed. The person card required a full elapsed day before it would say
// overdue, while the task list, every backend surface and the SQL all flip the
// instant the moment passes — so for a whole day the same promise read "due
// yesterday" on the contact and "overdue" on the task list, and a reader had no
// way to tell which surface was lying.
//
// The count and the verdict are DIFFERENT QUESTIONS, which is why both come
// back and only one of them decides. Something hours past its date is late by
// no whole days, and zero whole days is not the same as not being late — a
// caller that reads the count as the verdict rebuilds exactly the bug this
// module exists to remove. Read `late`; use `days` for wording only.
//
// Strictly past, which is what `shared/kernel/deadline.Passed` answers and what
// the SQL asks (`due_at < now()`): at the instant a promise falls due it is
// due, not late, and a reader shown "overdue" in that instant is told they have
// already failed something they have not.
//
// Whole days ELAPSED, not calendar days crossed — a promise due at 23:00 and
// read at 01:00 has crossed a calendar day and is two hours late, which is not
// "a day late". Counting the days a reader would name needs the zone they keep
// them in; that is format/calendarday's question and it takes one.

const MS_PER_DAY = 86_400_000;

export type Lateness = Readonly<{
  /** Whole days elapsed since the due moment. Zero while `late` is false. */
  days: number;
  /** The verdict: the due moment is behind us. */
  late: boolean;
}>;

/**
 * How far past a due moment `now` is, in epoch milliseconds on both sides.
 *
 * An unreadable due instant — `Date.parse` of a malformed wire string is NaN —
 * is reported not late rather than late by a NaN number of days: a verdict
 * nothing supports is a verdict this module does not give.
 */
export function daysPast(dueMs: number, nowMs: number): Lateness {
  if (Number.isNaN(dueMs) || dueMs >= nowMs) {
    return { days: 0, late: false };
  }
  return { days: Math.floor((nowMs - dueMs) / MS_PER_DAY), late: true };
}
