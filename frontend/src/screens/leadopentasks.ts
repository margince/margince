import { formatNumber } from "../format/format";
import { type Locale, translatePlural } from "../i18n";

// The open-task count beside a lead's next step, in the singular for one.
export function openTaskCountLabel(locale: Locale, count: number): string {
  return translatePlural(locale, "lead.openTaskCount", count, {
    count: formatNumber(count, locale),
  });
}
