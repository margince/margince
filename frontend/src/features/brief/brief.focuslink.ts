// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect } from "react";
import { replaceDial } from "../../lib/urlstate";
import type { Worklist, WorklistItem } from "../worklist/worklist.queries";

/**
 * The Home address dial that opens one row of the day, as a click on it does.
 * Mail links each row it lists here (mailcopy.WorkItemFragment).
 */
export const FOCUS_PARAM = "focus";

/** One row's key: its lane and id, since ids repeat across lanes. */
export function focusKey(item: Pick<WorklistItem, "source" | "id">): string {
  return `${item.source}-${item.id}`;
}

/** Every row Home has loaded: the day's focus, then each queue page. */
export function dayRows(
  day: Readonly<{ focus?: { items: WorklistItem[] } }> | undefined,
  pages: readonly Pick<Worklist, "queue">[] | undefined,
): WorklistItem[] {
  return [
    ...(day?.focus?.items ?? []),
    ...(pages?.flatMap((page) => page.queue) ?? []),
  ];
}

/** The loaded row with this key, if Home still lists it. */
export function rowByKey(
  rows: readonly WorklistItem[],
  key: string | undefined,
): WorklistItem | undefined {
  return key ? rows.find((item) => focusKey(item) === key) : undefined;
}

/**
 * Clear a focus dial whose row the loaded day no longer lists, so a later
 * refresh that lists it again does not open it unasked.
 */
export function useStaleFocusCleared(
  key: string | undefined,
  found: WorklistItem | undefined,
  loaded: boolean,
): void {
  useEffect(() => {
    if (key && loaded && !found) replaceDial(FOCUS_PARAM, undefined);
  }, [key, found, loaded]);
}
