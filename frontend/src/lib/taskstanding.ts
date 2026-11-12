// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { calendarDaysUntil } from "../format/daysuntil";
import { calendarLateness } from "../format/lateness";

/** What a task's standing is read from: its due date, and whether it is late. */
export type DatedTask = Readonly<{ due_at?: string | null; overdue: boolean }>;

// A task due within this many days reads in warning ink.
const SOON_DAYS = 7;

/**
 * Where an open task stands at `as_of`: late by calendar days, due soon, due
 * later on a named day, or undated. The company and contact Overview cards read
 * the same answer.
 */
export type TaskStanding =
  | { kind: "late"; days?: number }
  | { kind: "today" }
  | { kind: "soon"; days: number }
  | { kind: "later"; days: number; due: string }
  | { kind: "undated" };

export function taskStanding(
  step: DatedTask,
  asOf: string,
  zone: string,
): TaskStanding {
  if (!step.due_at) {
    return { kind: "undated" };
  }
  if (step.overdue) {
    const { days } = calendarLateness(step.due_at, asOf, zone);
    return { kind: "late", days: days > 0 ? days : undefined };
  }
  const days = calendarDaysUntil(step.due_at, zone, new Date(asOf));
  if (days <= 0) {
    return { kind: "today" };
  }
  return days <= SOON_DAYS
    ? { kind: "soon", days }
    : { kind: "later", days, due: step.due_at };
}
