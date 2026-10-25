// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { calendarDaysUntil } from "../../format/daysuntil";
import { daysLate } from "./companynextbar";

// What the four Overview cards list, read off the 360 alone and dated by its
// `as_of`, never by the clock.

type Company360 = components["schemas"]["Company360"];
type NextStep = NonNullable<Company360["next_steps"]>["data"][number];
type Activity = NonNullable<Company360["activities"]>["data"][number];
type Deal = NonNullable<Company360["deals"]>["data"][number];
type Project = NonNullable<Company360["projects"]>[number];
type Contact = NonNullable<Company360["contacts"]>["data"][number];
type Pipeline = components["schemas"]["Pipeline"];

// Each card is a glance; its link opens the tab that holds the whole list.
export const OVERVIEW_ROWS = 3;
export const OVERVIEW_CONTACTS = 2;
// A task due within this many days reads in warning ink.
const SOON_DAYS = 7;

/**
 * Where an open task stands at `as_of`: late by whole days, due soon, due
 * later on a named day, or undated. The Tasks tab's table and the Overview
 * card read the same answer.
 */
export type TaskStanding =
  | { kind: "late"; days?: number }
  | { kind: "today" }
  | { kind: "soon"; days: number }
  | { kind: "later"; days: number; due: string }
  | { kind: "undated" };

export function taskStanding(
  step: NextStep,
  asOf: string,
  zone: string,
): TaskStanding {
  if (!step.due_at) {
    return { kind: "undated" };
  }
  if (step.overdue) {
    return { kind: "late", days: daysLate(step, asOf, zone) };
  }
  const days = calendarDaysUntil(step.due_at, zone, new Date(asOf));
  if (days <= 0) {
    return { kind: "today" };
  }
  return days <= SOON_DAYS
    ? { kind: "soon", days }
    : { kind: "later", days, due: step.due_at };
}

/** The open tasks the Next bar does not already draw. */
export function otherOpenTasks(
  view: Company360,
  shownTaskId: string | undefined,
): NextStep[] {
  return (view.next_steps?.data ?? [])
    .filter((step) => step.activity_id !== shownTaskId)
    .slice(0, OVERVIEW_ROWS);
}

// A meeting that never took place is not something that happened here.
const DID_NOT_HAPPEN = new Set(["canceled", "no_show"]);

/** The newest exchanges that already happened, newest first. Tasks and
 *  booked meetings ahead are not exchanges yet. */
export function latestExchanges(view: Company360): Activity[] {
  const asOf = Date.parse(view.as_of);
  return (view.activities?.data ?? [])
    .filter(
      (row) =>
        row.kind !== "task" &&
        !DID_NOT_HAPPEN.has(row.meeting_status ?? "") &&
        Date.parse(row.occurred_at) <= asOf,
    )
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))
    .slice(0, OVERVIEW_ROWS);
}

export type WorkRow =
  | { kind: "deal"; deal: Deal }
  | { kind: "project"; project: Project };

/** The work in flight: open deals first, then the projects. */
export function workInFlight(view: Company360): WorkRow[] {
  const deals = (view.deals?.data ?? [])
    .filter((deal) => deal.status === "open")
    .map((deal): WorkRow => ({ kind: "deal", deal }));
  const projects = (view.projects ?? [])
    .filter((project) => project.phase !== "closed")
    .map((project): WorkRow => ({ kind: "project", project }));
  return [...deals, ...projects].slice(0, OVERVIEW_ROWS);
}

/**
 * Which of its pipeline's open stages a deal stands on, counted by position.
 * Nothing when the stage is not found among the pipelines read, so the row
 * never shows a guessed "stage N of M".
 */
export function stageRank(
  stageId: string | null | undefined,
  pipelines: readonly Pipeline[] | undefined,
): { rank: number; of: number } | undefined {
  if (!stageId || !pipelines) {
    return undefined;
  }
  for (const pipeline of pipelines) {
    const open = (pipeline.stages ?? [])
      .filter((stage) => stage.semantic === "open")
      .sort((a, b) => a.position - b.position);
    const at = open.findIndex((stage) => stage.id === stageId);
    if (at >= 0) {
      return { rank: at + 1, of: open.length };
    }
  }
  return undefined;
}

/** The contacts the relationship is strongest with, strongest first. */
export function strongestContacts(view: Company360): Contact[] {
  return [...(view.contacts?.data ?? [])]
    .sort((a, b) => b.strength.score - a.strength.score)
    .slice(0, OVERVIEW_CONTACTS);
}

/**
 * Whether the account has an open deal and nobody on file holds its economic
 * buyer seat. A roster the reader cannot see in full answers nothing.
 */
export function economicBuyerMissing(view: Company360): boolean {
  const roster = view.contacts;
  if (!roster || roster.page.has_more) {
    return false;
  }
  const open = (view.deals?.data ?? []).some((deal) => deal.status === "open");
  return (
    open &&
    !roster.data.some((contact) =>
      contact.deal_roles.some((seat) => seat.role === "economic_buyer"),
    )
  );
}
