// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDate } from "../format/format";
import type { Locale, Translator } from "../i18n";
import type { WorklistItem } from "./worklist.queries";

export function leadFactsText(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
): string | null {
  const lead = item.lead;
  if (!lead) return null;
  const parts: string[] = [];
  if (lead.company_name) parts.push(lead.company_name);
  const statuses = {
    new: "lead.statusNew",
    contacted: "lead.statusContacted",
    engaged: "lead.statusEngaged",
    promoted: "lead.statusPromoted",
    disqualified: "lead.statusDisqualified",
  } as const;
  switch (lead.status) {
    case "new":
    case "contacted":
    case "engaged":
    case "promoted":
    case "disqualified":
      parts.push(t(statuses[lead.status]));
  }
  if (lead.source) parts.push(lead.source);
  if (lead.last_activity_at)
    parts.push(
      t("worklist.lead.lastTouch", {
        date: formatDate(lead.last_activity_at, locale, zone),
      }),
    );
  if (lead.response_target_tracked === false)
    parts.push(t("worklist.lead.noTarget"));
  return parts.length > 0 ? parts.join(" · ") : null;
}
