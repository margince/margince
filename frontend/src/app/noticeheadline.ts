// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import { splitStagedSummary } from "../lib/stagedsummary";

/**
 * What a notice is headed with.
 *
 * An approval notice carries the staged change's summary as its subject: a
 * sentence, then the change's raw field pairs. The sentence heads the row and
 * the pairs stay as detail. With no sentence, the row is headed by what the
 * notice is.
 */
export function noticeHeadline(
  notice: Readonly<{ kind: string; subject: string }>,
  t: Translator,
): { headline: string; detail?: string } {
  if (notice.kind !== "approval_pending") {
    return { headline: notice.subject };
  }
  const { sentence, fields } = splitStagedSummary(notice.subject);
  const headline = sentence ?? t("notifications.class.approval_pending.label");
  return fields ? { headline, detail: fields } : { headline };
}
