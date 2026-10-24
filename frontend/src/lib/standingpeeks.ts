// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { PeekContent, PeekItem } from "../design-system/peek";
import { formatNumber } from "../format/format";
import { formatDayMonth } from "../format/format.dates";
import type { Locale, PluralTranslator, Translator } from "../i18n";

/** How the pieces of a peek are worded on a record page. */
export type PeekWords = Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
}>;

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

function taskDue(
  task: PeekTask,
  { t, locale, zone }: PeekWords,
): string | undefined {
  if (!task.dueAt) {
    return t("standing.next.peek.undated");
  }
  const day = formatDayMonth(task.dueAt, locale, zone);
  return task.overdue ? t("standing.next.peek.overdue", { date: day }) : day;
}
