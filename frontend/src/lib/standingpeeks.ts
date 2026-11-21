// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { PeekContent, PeekItem } from "../design-system/peek";
import { taskDueOnDay, taskDueZone } from "../format/calendarday";
import { formatNumber } from "../format/format";
import { formatDayMonth } from "../format/format.dates";
import type { Locale, PluralTranslator, Translator } from "../i18n";

/** How the pieces of a peek are worded on a record page. */
export type PeekWords = Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
  /** The company's zone, which counts a task that has only a date. */
  companyZone?: string;
}>;

/**
 * A task's due day on a record page, as the instant to read and the zone to
 * read it in. A timed task reads on the reader's clock. A task with only a date
 * reads as its picked day on the company's calendar.
 */
export function dueOnDay(
  words: Readonly<{ zone: string; companyZone?: string }>,
  due: string,
  hasTime: boolean | undefined,
  intent: components["schemas"]["IntentTime"] | undefined,
): { at: string; zone: string } {
  const zone = taskDueZone(
    hasTime,
    words.zone,
    words.companyZone ?? words.zone,
  );
  return { at: taskDueOnDay(due, hasTime, intent, zone), zone };
}

/**
 * The closing line of a card whose rows are a page of a longer list. It stops
 * a count of eleven sitting over a card of six that passes for all of them.
 * Nothing when the rows are the whole.
 */
export function partialFoot(
  shown: number,
  total: number,
  { plural, locale }: PeekWords,
): string | undefined {
  return total > shown
    ? plural("standing.peek.partial", total, {
        shown: formatNumber(shown, locale),
        count: formatNumber(total, locale),
      })
    : undefined;
}

/** One open task as the card under "+N tasks" lists it. */
export type PeekTask = Readonly<{
  id: string;
  subject: string;
  dueAt?: string | null;
  /** A task with only a date prints that date in every zone. */
  dueHasTime?: boolean;
  dueIntent?: components["schemas"]["IntentTime"];
  overdue?: boolean;
}>;

/**
 * The tasks behind the Next bar's "+N tasks" link, nearest first as the page
 * ordered them. A row opens the task where the page can, else the card is a
 * list to read.
 */
export function tasksPeek(
  tasks: readonly PeekTask[],
  words: PeekWords,
  total: number,
  onOpen?: (id: string) => void,
): PeekContent | undefined {
  if (tasks.length === 0) {
    return undefined;
  }
  const items: PeekItem[] = tasks.map((task) => ({
    key: task.id,
    title: task.subject,
    sub: taskDue(task, words),
    onOpen: onOpen && (() => onOpen(task.id)),
  }));
  return {
    head: words.t("standing.next.peek"),
    items,
    foot: partialFoot(tasks.length, total, words),
  };
}

function taskDue(task: PeekTask, words: PeekWords): string | undefined {
  const { t, locale } = words;
  if (!task.dueAt) {
    return t("standing.next.peek.undated");
  }
  const due = dueOnDay(words, task.dueAt, task.dueHasTime, task.dueIntent);
  const day = formatDayMonth(due.at, locale, due.zone);
  return task.overdue ? t("standing.next.peek.overdue", { date: day }) : day;
}
