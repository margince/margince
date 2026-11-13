// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { briefDay } from "../features/brief/brief.facts";
import { waitingRows } from "../features/brief/brief.sentence";
import type { WorklistItem } from "../features/worklist/worklist.queries";
import { useWorklist } from "../features/worklist/worklist.reads";
import { formatNumber } from "../format/format";
import {
  type Locale,
  type PluralTranslator,
  useLocale,
  usePlural,
} from "../i18n";
import { type Route, useRoute } from "../lib/router";
import { DEFAULT_VIEW } from "../lib/worklistviews";
import { plain, type SpokenLine } from "./ai-activity-speak";

/**
 * The rows of the reader's day the agent prepared or found, as Home lists them
 * under "Needs you". The same read with the same key, so on Home the two share
 * one request and can never count differently.
 */
export function useReadyRows(): readonly WorklistItem[] | undefined {
  const query = useWorklist("mine", "all", undefined, true, DEFAULT_VIEW);
  const day = briefDay(query.data?.pages);
  return day ? waitingRows(day) : undefined;
}

/** The record screens a route can name one record on. */
const RECORD_SCREENS: ReadonlySet<Route["screen"]> = new Set([
  "contacts",
  "companies",
  "partners",
  "leads",
  "deals",
  "projects",
]);

/**
 * The rows about the record the reader has open: its subject, the contact it
 * is with, or that contact's account. Empty off a record page.
 */
export function rowsAbout(
  rows: readonly WorklistItem[],
  route: Route,
): readonly WorklistItem[] {
  const id = route.id;
  if (id === undefined || !RECORD_SCREENS.has(route.screen)) return [];
  return rows.filter(
    (row) =>
      row.subject?.id === id ||
      row.contact?.id === id ||
      row.contact?.employer?.company_id === id ||
      row.company?.id === id,
  );
}

/** The day's rows with the open record's first, the order the panel lists. */
export function aboutFirst(
  rows: readonly WorklistItem[],
  about: readonly WorklistItem[],
): readonly WorklistItem[] {
  return about.length === 0
    ? rows
    : [...about, ...rows.filter((row) => !about.includes(row))];
}

/**
 * What the orb says at rest when something waits: how much. On a record page
 * it says how many are about that record, because the record shows its own.
 * The first row's name is the peek's, so the rail holds one short line.
 */
export function readyLine(
  rows: readonly WorklistItem[],
  about: readonly WorklistItem[],
  plural: PluralTranslator,
  locale: Locale,
): SpokenLine | null {
  if (rows.length === 0) return null;
  const count = formatNumber(rows.length, locale);
  return plain(
    about.length > 0
      ? plural("agent.ready.about", about.length, {
          about: formatNumber(about.length, locale),
          count,
        })
      : plural("agent.ready.count", rows.length, { count }),
  );
}

export type AgentReady = Readonly<{
  /** The day's rows, the open record's first; undefined until the day is read. */
  rows: readonly WorklistItem[] | undefined;
  /** How many of them are about the open record. */
  about: number;
  line: SpokenLine | null;
}>;

/** Everything the orb says and draws about the reader's day, read once. */
export function useAgentReady(): AgentReady {
  const day = useReadyRows();
  const route = useRoute();
  const plural = usePlural();
  const { locale } = useLocale();
  if (day === undefined) return { rows: undefined, about: 0, line: null };
  const about = rowsAbout(day, route);
  return {
    rows: aboutFirst(day, about),
    about: about.length,
    line: readyLine(day, about, plural, locale),
  };
}
