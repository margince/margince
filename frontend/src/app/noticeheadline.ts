// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import {
  summaryFieldsLine,
  summarySentence,
} from "../design-system/approvalsummary";
import type { Translator } from "../i18n";
import { summaryWords } from "../lib/approvalpreviewfields";

type NotificationItem = components["schemas"]["NotificationItem"];

/**
 * What a notice is headed with.
 *
 * An approval notice carries the staged change's summary as its subject, and
 * the summary taken apart beside it. The sentence heads the row and the field
 * values are the detail, worded as the approval card words them. A notice
 * written before the parts were kept is headed by its whole subject.
 */
export function noticeHeadline(
  notice: Pick<NotificationItem, "kind" | "subject" | "summary_parts">,
  t: Translator,
): { headline: string; detail?: string } {
  if (notice.kind !== "approval_pending") {
    return { headline: notice.subject };
  }
  const headline =
    summarySentence({
      summary: notice.subject,
      summary_parts: notice.summary_parts,
    }) ?? t("notifications.class.approval_pending.label");
  const detail = summaryFieldsLine(notice.summary_parts, summaryWords(t));
  return detail ? { headline, detail } : { headline };
}
