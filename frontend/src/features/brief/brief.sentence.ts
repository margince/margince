// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../../format/format";
import { type Locale, pluralKey } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Worklist, WorklistItem } from "../worklist/worklist.queries";
import {
  boundedCategories,
  laneUnread,
  scopeWasCut,
} from "./brief.readings.honesty";

/**
 * What the sentence says, as a key and the parts to fill it with.
 *
 * A key rather than a built string: this composes in three languages, and a
 * function that concatenated clauses would produce German with English word
 * order. The translator gets a whole sentence with named holes.
 */
export type BriefSentence = Readonly<{
  key: MessageKey;
  values: Readonly<Record<string, string>>;
}>;

/** The agenda includes approvals, using the worklist's existing review action. */
export function waitingRows(
  day: Worklist | undefined,
): readonly WorklistItem[] {
  return day?.focus?.items ?? [];
}

/**
 * The day in one sentence, or null when there is nothing true to say.
 *
 * Null on an unread day, deliberately. A page that said "nothing needs you"
 * over a read that never landed would send a rep away believing their morning
 * was clear — the same distinction the section below keeps between an empty
 * queue and a failed read.
 */
export function briefSentence(
  day: Worklist | undefined,
  t: (key: MessageKey, values?: Record<string, string>) => string,
  locale: Locale,
): BriefSentence | null {
  if (
    !day?.focus ||
    day.readings?.more_available ||
    laneUnread(day) ||
    scopeWasCut(day) ||
    boundedCategories(day).size > 0 ||
    day.focus.total > day.focus.items.length
  )
    return null;
  const count = day.focus.total;
  if (count === 0) return null;
  const groups = new Map<WorklistItem["category"], number>();
  for (const row of day.focus.items) {
    groups.set(row.category, (groups.get(row.category) ?? 0) + 1);
  }
  const agenda = [];
  for (const [category, total] of groups) {
    const key = AGENDA_CATEGORY[category];
    if (!key) return null;
    agenda.push(
      t(pluralKey(locale, key, total), { count: formatNumber(total, locale) }),
    );
  }
  return {
    key: "brief.agenda.summary",
    values: { agenda: agenda.join(" · ") },
  };
}

const AGENDA_CATEGORY = {
  customer_waiting: "brief.agenda.customer_waiting",
  leads: "brief.agenda.leads",
  deals_at_risk: "brief.agenda.deals_at_risk",
  meetings: "brief.agenda.meetings",
  tasks: "brief.agenda.tasks",
  decisions: "brief.agenda.decisions",
  relationships: "brief.agenda.relationships",
  system: "brief.agenda.system",
  // Focus never admits work due after today; one that did would be a task.
  upcoming: "brief.agenda.tasks",
} as const;
