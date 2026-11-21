import type { Locale } from "../i18n";
import { numberFormat } from "./formatters";

// formatAiRunCost renders an AI run's cost, reported in micro-USD. One read
// often costs a fraction of a cent, which formatMoney would show as $0.00.
export function formatAiRunCost(microusd: number, locale: Locale): string {
  return numberFormat(locale, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: microusd > 0 && microusd < 10_000 ? 4 : 2,
    maximumFractionDigits: 6,
  }).format(microusd / 1_000_000);
}
