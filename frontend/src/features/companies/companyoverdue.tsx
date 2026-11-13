import {
  type OverdueReading,
  overdueReadingOf,
  useFinanceSummary,
} from "../../components/financesummary";
import { Callout } from "../../design-system/callout";
import { formatDate, formatMoney, formatNumber } from "../../format/format";
import {
  type Locale,
  type PluralTranslator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

// The account warning for the company page band. A hook rather than a
// component: with nothing to warn of, the page passes no band, which leaves no
// empty gap.
// An archived account asks for no summary: a past customer carries no
// overdue figure.
export function useOverdueWarning(
  companyId: string,
  archived: boolean,
): OverdueReading | null {
  const { data } = useFinanceSummary(companyId, !archived);
  return overdueReadingOf(data);
}

// Everyone who works the account sees the same warning. The words come from
// the accounting source alone. A figure from a stale or failed sync is said in
// the past and dated, since an invoice may have been paid since.
export function OverdueWarning({
  reading,
}: Readonly<{ reading: OverdueReading }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const lines: string[] = [];
  if (reading.asOf) {
    lines.push(
      t("co.overdue.asOf", { date: formatDate(reading.asOf, locale, zone) }),
    );
  }
  if (reading.oldestDays) {
    lines.push(
      plural("co.overdue.oldest", reading.oldestDays, {
        days: formatNumber(reading.oldestDays, locale),
      }),
    );
  }
  // Zero reminders is a fact the source states, and it is no warning.
  if (reading.dunningLevel) {
    lines.push(
      plural("co.overdue.reminders", reading.dunningLevel, {
        count: formatNumber(reading.dunningLevel, locale),
      }),
    );
  }
  return (
    <Callout
      tone={reading.asOf ? "info" : "warning"}
      kind="standing"
      title={warningTitle(reading, locale, t, plural)}
    >
      {lines.length > 0 ? lines.join(" ") : undefined}
    </Callout>
  );
}

// An older server sends the amount without a count, and the title then names
// the amount alone rather than the warning disappearing.
function warningTitle(
  reading: OverdueReading,
  locale: Locale,
  t: ReturnType<typeof useT>,
  plural: PluralTranslator,
): string {
  const amount = formatMoney(reading.amountMinor, reading.currency, locale);
  if (reading.count == null) {
    return t(
      reading.asOf ? "co.overdue.amountOnlyAsOf" : "co.overdue.amountOnly",
      { amount },
    );
  }
  return plural(
    reading.asOf ? "co.overdue.titleAsOf" : "co.overdue.title",
    reading.count,
    { amount, count: formatNumber(reading.count, locale) },
  );
}
