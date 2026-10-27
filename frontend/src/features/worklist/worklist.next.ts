// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Route } from "../../lib/router";
import { NO_URL_PARAMS, type UrlParams } from "../../lib/urlstate";
import { drawerQueue } from "./worklist.address";
import { rowHref } from "./worklist.copy";
import type { WorklistItem } from "./worklist.queries";
import {
  returningRecord,
  withWorklistReturn,
  worklistReturn,
} from "./worklist.return";

/**
 * The Worklist a record continues: the one its address came from, else the
 * reader's own whole list. `carried` is what the next record's address keeps.
 */
export function continuedList(params: UrlParams) {
  const carried = worklistReturn(params) ?? NO_URL_PARAMS;
  return { ...drawerQueue(carried), carried };
}

/** A record with a row in the list, and whether it is the one on screen. */
type Stop = Readonly<{ href: string; here: boolean }>;

/** Each record once, where its first row is; rows off record pages drop out. */
function recordsInOrder(
  rows: readonly WorklistItem[],
  onScreen: Route,
): Stop[] {
  const seen = new Map<string, Stop>();
  for (const row of rows) {
    const href = rowHref(row);
    const record = href && returningRecord(href);
    const key = record && `${record.screen}/${record.id}`;
    if (href && record && key && !seen.has(key)) {
      seen.set(key, {
        href,
        here: record.screen === onScreen.screen && record.id === onScreen.id,
      });
    }
  }
  return [...seen.values()];
}

/**
 * The record after the one on screen, in the order records first appear.
 * Once the list has `ended` it wraps to the first; before then, undefined
 * means read on.
 */
export function nextTaskHref(
  rows: readonly WorklistItem[],
  onScreen: Route,
  carried: UrlParams,
  ended: boolean,
): string | undefined {
  const records = recordsInOrder(rows, onScreen);
  const here = records.findIndex((record) => record.here);
  const target =
    (here >= 0 ? records[here + 1] : undefined) ??
    (ended ? records.find((record) => !record.here) : undefined);
  return target && withWorklistReturn(target.href, carried);
}
