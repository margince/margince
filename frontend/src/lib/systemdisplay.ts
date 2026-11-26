// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { effectiveDisplayLocale } from "../i18n/locale";

type Display = components["schemas"]["SystemDisplay"];
type Presentable = { system_display?: Display };
export function displayedSubject<
  T extends Presentable & { subject?: string | null },
>(record: T): string | null | undefined {
  return matching(record)?.subject ?? record.subject;
}
export function displayedBody<T extends Presentable & { body?: string | null }>(
  record: T,
): string | null | undefined {
  return matching(record)?.body ?? record.body;
}
export function displayedTitle<
  T extends Presentable & { title?: string | null },
>(record: T): string | null | undefined {
  return matching(record)?.subject ?? record.title;
}
function matching(record: Presentable): Display | undefined {
  const display = record.system_display;
  return display?.locale === effectiveDisplayLocale() ? display : undefined;
}
