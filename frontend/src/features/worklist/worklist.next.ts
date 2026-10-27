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

/** The address of the first row on another record than the one on screen, or undefined. */
export function nextTaskHref(
  rows: readonly WorklistItem[],
  onScreen: Route,
  carried: UrlParams,
): string | undefined {
  for (const row of rows) {
    const href = rowHref(row);
    const record = href && returningRecord(href);
    if (
      href &&
      record &&
      (record.screen !== onScreen.screen || record.id !== onScreen.id)
    ) {
      return withWorklistReturn(href, carried);
    }
  }
  return undefined;
}
