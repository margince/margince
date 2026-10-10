// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";

// The staged-change summary joins `field=value` pairs with ", " (summaryFields
// in the server's compose/agentsummary.go). noticeheadline.test.ts holds this
// pattern against the server's own output.
export const FIELD_PAIRS = /^[a-z][a-z0-9_]*=[^,]*(, [a-z][a-z0-9_]*=[^,]*)*$/;

/**
 * What a notice is headed with.
 *
 * An approval notice carries the staged change's summary as its subject. With
 * no sentence in front, that is only the change's fields, which tell a reader
 * nothing unopened. The row is then headed by what the notice is, with the
 * fields kept as detail.
 */
export function noticeHeadline(
  notice: Readonly<{ kind: string; subject: string }>,
  t: Translator,
): { headline: string; detail?: string } {
  if (notice.kind !== "approval_pending" || !FIELD_PAIRS.test(notice.subject)) {
    return { headline: notice.subject };
  }
  return {
    headline: t("notifications.class.approval_pending.label"),
    detail: notice.subject,
  };
}
