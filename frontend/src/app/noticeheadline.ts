// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Translator } from "../i18n";

type NotificationClass = components["schemas"]["NotificationClass"];

// A Record over the contract's union: a class the server adds fails the build
// here until it has a place in the preference words this reads.
const CLASSES: Record<NotificationClass, true> = {
  automation: true,
  lead_sla: true,
  approval_pending: true,
  capture: true,
  coach: true,
  system: true,
};

function isClass(kind: string): kind is NotificationClass {
  return Object.hasOwn(CLASSES, kind);
}

// "into_tag_id=01a1…, to_stage_id=01a0…": field names and ids, no sentence.
const FIELD_PAIRS = /^[a-z][a-z0-9_]*=[^,]*(, [a-z][a-z0-9_]*=[^,]*)*$/;

/**
 * What a notice is headed with.
 *
 * The server's subject is a sentence for most notices, and for a staged change
 * that has none it is the change's own fields. Those say nothing to a reader
 * who has not opened the notice, so the class (the words the preferences page
 * already uses) heads the row and the fields stay beneath as detail.
 */
export function noticeHeadline(
  notice: Readonly<{ kind: string; subject: string }>,
  t: Translator,
): { headline: string; detail?: string } {
  if (!isClass(notice.kind) || !FIELD_PAIRS.test(notice.subject)) {
    return { headline: notice.subject };
  }
  return {
    headline: t(`notifications.class.${notice.kind}.label`),
    detail: notice.subject,
  };
}
