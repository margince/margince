import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Waterfall, type WaterfallStep } from "../../design-system/waterfall";
import { formatMoney, formatMoneyOrAbsent } from "../../format/format";
import { formatMoneyOrWord } from "../../format/moneyword";
import { type Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Review = components["schemas"]["WeeklyReview"];
type Outlook = NonNullable<Review["outlook"]>[number];
type Bar = Outlook["movement"][number];

// Where the week was landing, and how it got there.
//
// Both halves are FROZEN: the figures were copied when the week closed, so a
// re-run forecast never changes the record of what the week was.

type ForecastRow = Readonly<{
  key: string;
  label: string;
  basis?: string;
  minor: (outlook: Outlook) => number | undefined;
}>;

const PERIOD_LABEL: Readonly<Record<Outlook["period_kind"], MessageKey>> = {
  week: "brief.weekly.outlook.week",
  month: "brief.weekly.outlook.month",
  quarter: "brief.weekly.outlook.quarter",
};

/**
 * The forecast as one table: a row per figure and a column per period the
 * review froze. The bridge under it follows the quarter, the widest period.
 */
export function OutlookPanel({
  outlook,
  locale,
  onOpenForecast,
}: Readonly<{
  outlook: readonly Outlook[];
  locale: Locale;
  // Analytics' forecast section, where these figures are read. The caller
  // owns navigation, so the report and any other host can place it.
  onOpenForecast: () => void;
}>) {
  const t = useT();
  // A review written before a forecast was composed carries none. Words, not
  // zeros: an unforecast week and a week that landed on nothing differ.
  const [first] = outlook;
  if (first === undefined) {
    return <p className="t-caption">{t("brief.weekly.outlook.none")}</p>;
  }
  const measure = first.forward_measure;
  const rows: ForecastRow[] = [
    {
      key: "won",
      label: t("brief.weekly.outlook.won"),
      minor: (o) => o.won_minor,
    },
    {
      key: "commit",
      label: t("brief.weekly.outlook.commit"),
      minor: (o) => o.commit_minor,
    },
    {
      key: "best",
      label: t("brief.weekly.outlook.bestCase"),
      // A reader adding best case to commit would count the overlap twice.
      basis: t("brief.weekly.outlook.bestCaseDetail"),
      minor: (o) => o.best_case_minor,
    },
    {
      key: "weighted",
      label: t("brief.weekly.outlook.weighted"),
      minor: (o) => o.weighted_minor,
    },
    ...(outlook.some((o) => o.closing_landing_minor !== undefined)
      ? [
          {
            key: "landing",
            label: t("brief.weekly.outlook.landing"),
            // The measure it was read under, frozen with the week.
            basis: measure
              ? t(`brief.weekly.outlook.measure.${measure}`)
              : undefined,
            minor: (o: Outlook) => o.closing_landing_minor,
          },
        ]
      : []),
  ];
  // An unconvertible period says so in words rather than as a bare dash.
  const money = (o: Outlook, minor: number | undefined) =>
    minor === undefined
      ? "—"
      : formatMoneyOrWord(
          minor,
          o.base_currency,
          locale,
          t("format.notForecast"),
          formatMoney,
        );
  const bridged = outlook.find((o) => o.period_kind === "quarter") ?? first;
  return (
    <>
      <DataTable
        label={t("brief.weekly.outlook")}
        rowKey={(row: ForecastRow) => row.key}
        rows={rows}
        columns={[
          {
            key: "figure",
            header: t("brief.report.col.figure"),
            grow: true,
            render: (row) => (
              <>
                {row.label}
                {row.basis && (
                  <span className="brief-report-basis t-caption">
                    {row.basis}
                  </span>
                )}
              </>
            ),
          },
          ...outlook.map((o) => ({
            key: o.period_kind,
            header: t(PERIOD_LABEL[o.period_kind]),
            align: "end" as const,
            render: (row: ForecastRow) => money(o, row.minor(o)),
          })),
        ]}
      />
      <div className="brief-report-actions">
        <Button variant="ghost" onClick={onOpenForecast}>
          {t("brief.weekly.outlook.open")}
        </Button>
      </div>
      <BridgePanel outlook={bridged} locale={locale} />
    </>
  );
}

// The movement bridge: the two landings, and the bars between them.
function BridgePanel({
  outlook,
  locale,
}: Readonly<{ outlook: Outlook; locale: Locale }>) {
  const t = useT();
  const money = (minor: number) =>
    formatMoneyOrAbsent(minor, outlook.base_currency, locale);

  // No Monday snapshot means no opening to have moved FROM. Said rather than
  // drawn from zero, which would show a week that started from nothing.
  if (
    outlook.opening_landing_minor === undefined ||
    outlook.closing_landing_minor === undefined
  ) {
    return <p className="t-caption">{t("brief.weekly.bridge.noOpening")}</p>;
  }

  const steps: WaterfallStep[] = outlook.movement.map((bar: Bar) => ({
    key: bar.bar,
    label: t(`brief.weekly.bar.${bar.bar}`),
    value: bar.delta_minor,
    amount: money(bar.delta_minor),
  }));

  return (
    <Waterfall
      label={t("brief.weekly.bridge")}
      opening={{
        label: t("brief.weekly.bridge.opening"),
        value: outlook.opening_landing_minor,
        amount: money(outlook.opening_landing_minor),
      }}
      closing={{
        label: t("brief.weekly.bridge.closing"),
        value: outlook.closing_landing_minor,
        amount: money(outlook.closing_landing_minor),
      }}
      // Order is the server's: the bars come back in the bridge's own drawing
      // order, and re-sorting here would tell a different story than the one
      // the review froze.
      steps={steps}
      reconciliationWarning={t("brief.weekly.bridge.reconcile")}
    />
  );
}
