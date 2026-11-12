// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, StatCard } from "../../design-system/atoms";
import {
  SegmentBar,
  type SegmentBarParts,
  SegmentSwatch,
} from "../../design-system/segmentbar";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatAnalyticsMoney,
  formatDateTime,
  formatNumber,
} from "../../format/format";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import {
  periodLabel,
  type SalesProgress,
  type SalesProgressAmount,
  type SalesProgressGoal,
} from "./progress.queries";
import type { ReportingEvidenceRef } from "./reporting.model";

type OpenEvidence = ((reference: ReportingEvidenceRef) => void) | undefined;

/** An amount as the reader sees it, or the word for why there is none. */
export function amountText(
  amount: SalesProgressAmount,
  currency: string,
  locale: Locale,
  t: Translator,
): string {
  if (amount.status === "not_started") return t("progress.notStarted");
  if (amount.amount_minor == null) return t("progress.withheld");
  return formatAnalyticsMoney(amount.amount_minor, currency, locale);
}

/** A goal as the reader sees it: the amount, or the state that stands for it. */
export function goalText(
  goal: SalesProgressGoal,
  currency: string,
  locale: Locale,
  t: Translator,
): string {
  switch (goal.status) {
    case "set":
    case "zero":
    case "changed_outside_plan":
      return goal.amount_minor == null
        ? t("progress.goal.missing")
        : formatAnalyticsMoney(goal.amount_minor, currency, locale);
    case "incomplete":
      return t("progress.goal.incomplete");
    default:
      return t(`progress.goal.${goal.status}`);
  }
}

/**
 * The difference to the goal as plain text. A period still running has an
 * amount to go, never a miss; only a figure above the goal takes a colour.
 */
export function ProgressDelta({
  delta,
  currency,
  open,
}: Readonly<{ delta: number | undefined; currency: string; open: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  if (delta == null) return null;
  const amount = formatAnalyticsMoney(Math.abs(delta), currency, locale);
  let tone = "below";
  let text = t("progress.belowGoal", { amount });
  if (delta === 0) {
    tone = "above";
    text = t("progress.goalMet");
  } else if (delta > 0) {
    tone = "above";
    text = t("progress.aboveGoal", { amount });
  } else if (open) {
    tone = "open";
    text = t("progress.toGo", { amount });
  }
  return (
    <span className="progress-delta t-num" data-tone={tone}>
      {text}
    </span>
  );
}

// The goal line under the headline figure, with the subtotal an incomplete
// goal still has. A team month counts as set only when every seller has it.
function goalLine(
  progress: SalesProgress,
  locale: Locale,
  t: Translator,
): string {
  const { goal, currency } = progress;
  if (goal.status === "incomplete" && goal.amount_minor != null) {
    const subtotal = formatAnalyticsMoney(goal.amount_minor, currency, locale);
    if (progress.scope.kind !== "owner")
      return t("progress.goalIncompleteTeamLine", { subtotal });
    return t("progress.goalIncompleteLine", {
      set: formatNumber(goal.months_set, locale),
      needed: formatNumber(goal.months_needed, locale),
      subtotal,
    });
  }
  if (goal.amount_minor != null)
    return t("progress.ofGoalLine", {
      goal: formatAnalyticsMoney(goal.amount_minor, currency, locale),
    });
  return t("progress.goalLine", { goal: goalText(goal, currency, locale, t) });
}

/** The sales won in the period against its goal, over the bar that shows why. */
export function ProgressResult({
  progress,
  onEvidence,
  onGoals,
}: Readonly<{
  progress: SalesProgress;
  onEvidence: OpenEvidence;
  onGoals: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { closed, goal, completion, currency } = progress;
  const started = progress.state !== "not_started";
  return (
    <section className="progress-hero" aria-label={t("progress.closedRevenue")}>
      <div className="progress-result">
        <span className="progress-result-label">
          {t("progress.closedRevenue")}
        </span>
        <p className="progress-result-figure">
          {amountText(closed, currency, locale, t)}
        </p>
        {goal.status === "missing" ? (
          <NoGoal progress={progress} onGoals={onGoals} />
        ) : (
          <p className="progress-result-goal">
            {goalLine(progress, locale, t)}
          </p>
        )}
        {(completion != null || progress.delta_minor != null) && (
          <p className="progress-result-line">
            {completion != null && (
              <span className="progress-result-completion">
                {t("progress.completion", {
                  percent: formatNumber(Math.round(completion * 100), locale),
                })}
              </span>
            )}
            <ProgressDelta
              delta={progress.delta_minor}
              currency={currency}
              open={progress.state !== "ended"}
            />
          </p>
        )}
        {closed.status === "partial" && (
          <p className="t-caption">{t("progress.partial")}</p>
        )}
        {goal.status === "changed_outside_plan" && (
          <p className="t-caption">{t("progress.goalChanged")}</p>
        )}
      </div>
      <ResultBar progress={progress} onEvidence={onEvidence} />
      {started && onEvidence && (
        <div className="progress-result-actions">
          <Button
            variant="link"
            onClick={() =>
              onEvidence({ metric: "bookings_won", context_id: "interval" })
            }
          >
            {t("progress.viewClosedDeals")}
          </Button>
        </div>
      )}
    </section>
  );
}

// The one place a reading with no goal says so. A reader who may set one gets
// the way to; any other reader is told who sets it.
function NoGoal({
  progress,
  onGoals,
}: Readonly<{ progress: SalesProgress; onGoals: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const own = progress.scope.kind === "owner";
  const sets = progress.capabilities.write_goals || !own;
  return (
    <div className="progress-nogoal">
      <p className="progress-result-goal">
        {t("progress.noGoalFor", {
          period: periodLabel(progress.period, locale),
        })}{" "}
        {!sets && t("progress.whoSetsGoals")}
      </p>
      {sets && (
        <Button variant="ghost" onClick={onGoals}>
          {t(own ? "progress.setGoals" : "progress.viewGoals")}
        </Button>
      )}
    </div>
  );
}

// Closed revenue as its sale categories, against the goal, with the planned
// open deals hatched after it: they may still close, and never count.
function ResultBar({
  progress,
  onEvidence,
}: Readonly<{ progress: SalesProgress; onEvidence: OpenEvidence }>) {
  const t = useT();
  const { locale } = useLocale();
  const { categories, closed, goal, planned, currency } = progress;
  const money = (minor: number) =>
    formatAnalyticsMoney(minor, currency, locale);
  const known =
    categories.status !== "withheld" && categories.status !== "not_started";
  const part = (key: string, label: string, minor: number) => ({
    key,
    label,
    value: minor,
    amount: money(minor),
  });
  const product = part(
    "product",
    t("progress.productSales"),
    categories.product_minor,
  );
  const service = part(
    "service",
    t("progress.serviceSales"),
    categories.service_minor,
  );
  const unresolved = part(
    "unresolved",
    t("progress.unclassified"),
    categories.unresolved_minor,
  );
  const whole = part(
    "closed",
    t("progress.closedRevenue"),
    closed.amount_minor ?? 0,
  );
  let parts: SegmentBarParts = [whole];
  if (known)
    parts =
      categories.unresolved_minor > 0
        ? [product, service, unresolved]
        : [product, service];
  const groups: Record<string, string | undefined> = {
    product: CATEGORY_GROUP.product,
    service: CATEGORY_GROUP.service,
  };
  // A withheld total has no length to draw; the headline already says so.
  if (closed.amount_minor == null) return null;
  return (
    <div className="progress-result-bar">
      <SegmentBar
        label={t("progress.readings")}
        legend="hidden"
        parts={parts}
        marker={
          goal.amount_minor != null && goal.status !== "incomplete"
            ? part("goal", t("progress.goal"), goal.amount_minor)
            : undefined
        }
        ahead={
          planned.amount_minor != null && planned.amount_minor > 0
            ? part("planned", t("progress.plannedAhead"), planned.amount_minor)
            : undefined
        }
        onSelect={
          onEvidence && known && closed.amount_minor != null
            ? (key) =>
                onEvidence({
                  metric: "bookings_mix",
                  context_id: "interval",
                  ...(groups[key] ? { group_key: groups[key] } : {}),
                })
            : undefined
        }
      />
    </div>
  );
}

// The commercial-mix groups each sale category reads from.
const CATEGORY_GROUP = {
  product: "kind:software",
  service: "kind:implementation",
} as const;

/** Product sales, services, planned revenue and what no line could place. */
export function ProgressReadings({
  progress,
  onEvidence,
}: Readonly<{ progress: SalesProgress; onEvidence: OpenEvidence }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { categories, currency, planned } = progress;
  const known =
    categories.status !== "withheld" && categories.status !== "not_started";
  const money = (minor: number) =>
    known
      ? formatAnalyticsMoney(minor, currency, locale)
      : amountText(progress.closed, currency, locale, t);
  const mix = (group?: string) =>
    onEvidence && known
      ? () =>
          onEvidence({
            metric: "bookings_mix",
            context_id: "interval",
            ...(group ? { group_key: group } : {}),
          })
      : undefined;
  return (
    <StatStrip label={t("progress.readings")} density="plate">
      <StatCard
        label={t("progress.productSales")}
        source={known && <SegmentSwatch step={0} />}
        value={money(categories.product_minor)}
        detail={itemsLine(categories.product_items, plural, locale)}
        onOpen={mix(CATEGORY_GROUP.product)}
        opens="records"
      />
      <StatCard
        label={t("progress.serviceSales")}
        source={known && <SegmentSwatch step={1} />}
        value={money(categories.service_minor)}
        detail={itemsLine(categories.service_items, plural, locale)}
        onOpen={mix(CATEGORY_GROUP.service)}
        opens="records"
      />
      <StatCard
        label={t("progress.planned")}
        source={
          planned.amount_minor != null &&
          planned.amount_minor > 0 && <SegmentSwatch step="ahead" />
        }
        value={amountText(planned, currency, locale, t)}
        detail={t("progress.plannedAt", {
          at: formatDateTime(
            progress.planned_valued_at,
            locale,
            progress.timezone,
          ),
        })}
        onOpen={
          onEvidence && planned.amount_minor != null
            ? () => onEvidence({ metric: "open_pipeline", context_id: "state" })
            : undefined
        }
        opens="records"
      />
      {known && categories.unresolved_minor > 0 && (
        <StatCard
          label={t("progress.unclassified")}
          source={<SegmentSwatch step={2} />}
          value={money(categories.unresolved_minor)}
          detail={
            categories.invalid_deals > 0
              ? plural("progress.invalidDeals", categories.invalid_deals, {
                  count: formatNumber(categories.invalid_deals, locale),
                })
              : t("progress.unclassifiedDetail")
          }
          tone="warning"
          onOpen={mix()}
          opens="records"
        />
      )}
    </StatStrip>
  );
}

// The first few names of what sold, most revenue first.
function itemsLine(
  items: readonly string[],
  plural: PluralTranslator,
  locale: Locale,
): string | undefined {
  if (items.length === 0) return undefined;
  const shown = items.slice(0, 3).join(", ");
  const rest = items.length - 3;
  return rest > 0
    ? plural("progress.itemsMore", rest, {
        items: shown,
        count: formatNumber(rest, locale),
      })
    : shown;
}
