import type { components } from "../api/schema";
import { SegmentedControl, StatCard } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { StatStrip } from "../design-system/statstrip";
import { Waterfall, type WaterfallStep } from "../design-system/waterfall";
import { formatMoneyOrAbsent } from "../format/format";
import { type Locale, useT } from "../i18n";

type Review = components["schemas"]["WeeklyReview"];
type Outlook = NonNullable<Review["outlook"]>[number];
type Bar = Outlook["movement"][number];

// Where the week was landing, and how it got there.
//
// Both halves are FROZEN. The figures were copied when the week closed, so this
// panel never re-reads a forecast: a retrospective that changed when somebody
// re-ran the numbers would stop being a record of what the week was.
export function OutlookPanel({
  outlook,
  locale,
  horizon,
  onHorizon,
}: Readonly<{
  outlook: readonly Outlook[];
  locale: Locale;
  horizon: string;
  onHorizon: (next: string) => void;
}>) {
  const t = useT();

  // A review written before a forecast was composed carries none. Said in
  // words, because a week nobody forecast and a week that landed on nothing
  // are different facts and zeros would claim the second.
  if (outlook.length === 0) {
    return (
      <Callout tone="info" title={t("brief.weekly.outlook")}>
        {t("brief.weekly.outlook.none")}
      </Callout>
    );
  }

  const shown =
    outlook.find((one) => one.period_kind === horizon) ?? outlook[0];
  const money = (minor: number) =>
    formatMoneyOrAbsent(minor, shown.base_currency, locale);

  return (
    <>
      <SegmentedControl
        label={t("brief.weekly.outlook")}
        value={shown.period_kind}
        onChange={onHorizon}
        // Only the horizons this review actually froze. Offering one it has no
        // figures for would give the reader a tab that draws nothing.
        options={outlook.map((one) => one.period_kind)}
        labels={{
          week: t("brief.weekly.outlook.week"),
          month: t("brief.weekly.outlook.month"),
          quarter: t("brief.weekly.outlook.quarter"),
        }}
      />

      <StatStrip>
        <StatCard
          label={t("brief.weekly.outlook.won")}
          value={money(shown.won_minor)}
          numeric
        />
        <StatCard
          label={t("brief.weekly.outlook.commit")}
          value={money(shown.commit_minor)}
          numeric
        />
        {/* The label says "incl. commit" because the figure includes it, and a
            reader adding best case to commit would double-count the overlap. */}
        <StatCard
          label={t("brief.weekly.outlook.bestCase")}
          value={money(shown.best_case_minor)}
          numeric
        />
        <StatCard
          label={t("brief.weekly.outlook.weighted")}
          value={money(shown.weighted_minor)}
          numeric
        />
        {shown.closing_landing_minor !== undefined && (
          <StatCard
            label={t("brief.weekly.outlook.landing")}
            value={money(shown.closing_landing_minor)}
            // Which measure produced it, because the same pipeline reads
            // differently under each and a landing with no basis is a number a
            // reader cannot argue with.
            detail={
              shown.forward_measure
                ? t(`brief.weekly.outlook.measure.${shown.forward_measure}`)
                : undefined
            }
            numeric
          />
        )}
      </StatStrip>

      <BridgePanel outlook={shown} locale={locale} />
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
    return (
      <Callout tone="info" title={t("brief.weekly.bridge")}>
        {t("brief.weekly.bridge.noOpening")}
      </Callout>
    );
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
