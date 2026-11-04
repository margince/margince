// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The one order every record's tab strip follows, set by the contact page.
 * A record shows only the tabs it has, in this order, and
 * `recordtaborder.test.ts` fails a list that breaks it.
 *
 * Linked addresses keep two tabs on two ids each: History is `timeline` or
 * `history`, and Documents is `documents` or `files`.
 */
export const RECORD_TAB_ORDER = [
  ["overview"],
  ["timeline", "history"],
  ["network"],
  ["contacts"],
  ["deals"],
  ["room"],
  ["meetings"],
  ["tasks"],
  ["finance"],
  ["research"],
  ["legal"],
  ["documents", "files"],
  ["profile"],
  ["partner"],
] as const;

/** RecordTabId is a tab id some record's strip may carry. */
export type RecordTabId = (typeof RECORD_TAB_ORDER)[number][number];
