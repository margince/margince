// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge, Button, StatCard } from "../../design-system/atoms";
import { SegmentBar, type SegmentBarParts } from "../../design-system/readings";
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
import type {
  SalesProgress,
  SalesProgressAmount,
  SalesProgressGoal,
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

/** The signed difference to the goal, in the tone of what it means. */
export function DeltaBadge({
  delta,
  currency,
}: Readonly<{ delta: number | undefined; currency: string }>) {
  const t = useT();
  const { locale } = useLocale();
  if (delta == null) return null;
  if (delta === 0) return <Badge>{t("progress.goalMet")}</Badge>;
  const amount = formatAnalyticsMoney(Math.abs(delta), currency, locale);
  return delta > 0 ? (
    <Badge tone="success">{t("progress.aboveGoal", { amount })}</Badge>
  ) : (
    <Badge tone="danger">{t("progress.belowGoal", { amount })}</Badge>
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
  return t("progress.goalLine", { goal: goalText(goal, currency, locale, t) });
}

/** The closed revenue of the period against its goal. */
export function ProgressResult({
  progress,
  onEvidence,
}: Readonly<{ progress: SalesProgress; onEvidence: OpenEvidence }>) {
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
        <p className="progress-result-goal">{goalLine(progress, locale, t)}</p>
        {completion != null && (
          <p className="progress-result-completion">
            {t("progress.completion", {
              percent: formatNumber(Math.round(completion * 100), locale),
            })}
          </p>
        )}
        {closed.status === "partial" && (
          <p className="t-caption">{t("progress.partial")}</p>
        )}
        {goal.status === "changed_outside_plan" && (
          <p className="t-caption">{t("progress.goalChanged")}</p>
        )}
        <div className="progress-result-actions">
          <DeltaBadge delta={progress.delta_minor} currency={currency} />
          {started && onEvidence && (
            <Button
              variant="link"
              onClick={() =>
                onEvidence({ metric: "bookings_won", context_id: "interval" })
              }
            >
              {t("progress.viewClosedDeals")}
            </Button>
          )}
        </div>
      </div>
      <ResultBar progress={progress} onEvidence={onEvidence} />
    </section>
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
  return (
    <div className="progress-result-bar">
      <SegmentBar
        label={t("progress.readings")}
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
        value={money(categories.product_minor)}
        detail={itemsLine(categories.product_items, plural, locale)}
        onOpen={mix(CATEGORY_GROUP.product)}
        opens="records"
      />
      <StatCard
        label={t("progress.serviceSales")}
        value={money(categories.service_minor)}
        detail={itemsLine(categories.service_items, plural, locale)}
        onOpen={mix(CATEGORY_GROUP.service)}
        opens="records"
      />
      <StatCard
        label={t("progress.planned")}
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
