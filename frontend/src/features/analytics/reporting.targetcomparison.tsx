import { StatCard } from "../../design-system/atoms";
import { Button } from "../../design-system/button";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { StatStrip } from "../../design-system/statstrip";
import { formatDecimal } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import {
  type ReportingEvaluation,
  type ReportingSelection,
  reportingCompactAmount,
} from "./reporting.model";

type Comparison = NonNullable<
  ReportingEvaluation["metrics"][number]["target_comparison"]
>;
type Qualified = NonNullable<Comparison["qualified"]>;

/**
 * Where this fiscal quarter's expected landing stands against its sales
 * target, as the server compared them. Every figure is the server's; this
 * only formats them and says why a figure is missing. `onApply` takes the
 * selection the server says it can compare, on a page that can switch to it.
 */
export function TargetComparison({
  evaluation,
  onApply,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onApply?: (selection: ReportingSelection) => void;
}>) {
  const t = useT();
  const comparison = evaluation.metrics.find(
    (metric) => metric.id === "forecast_landing",
  )?.target_comparison;
  if (!comparison) return null;
  return (
    <Panel title={t("reporting.compare.title")}>
      <PanelBody>
        {comparison.status === "compared" ||
        comparison.status === "target_reached" ? (
          <ComparedFigures
            comparison={comparison}
            currency={evaluation.context.currency}
          />
        ) : (
          <ComparisonState comparison={comparison} onApply={onApply} />
        )}
      </PanelBody>
    </Panel>
  );
}

function ComparedFigures({
  comparison,
  currency,
}: Readonly<{ comparison: Comparison; currency: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const amount = (value: number | undefined) =>
    reportingCompactAmount(value, currency, currency, locale);
  const gap = comparison.gap ?? 0;
  const reached = comparison.status === "target_reached";
  return (
    <StatStrip label={t("reporting.compare.title")}>
      <StatCard
        label={t("reporting.compare.gap")}
        value={amount(Math.abs(gap))}
        detail={t(gapWords(gap), { target: amount(comparison.target) })}
        basis={t("reporting.compare.gapBasis", {
          landing: amount(comparison.landing),
          basis: t(
            `reporting.compare.basis.${comparison.basis ?? "commit_evidence"}`,
          ),
        })}
      />
      <StatCard
        label={t("reporting.compare.remaining")}
        value={
          reached
            ? t("reporting.compare.reached")
            : amount(comparison.remaining)
        }
        detail={t("reporting.compare.remainingBasis", {
          won: amount(comparison.won),
          target: amount(comparison.target),
        })}
      />
      <StatCard
        label={t("reporting.compare.coverageAll")}
        value={coverageValue(comparison.coverage, reached, t, locale)}
        detail={t("reporting.compare.coverageBasis", {
          open: amount(comparison.open_pipeline),
        })}
      />
      <QualifiedCard
        qualified={comparison.qualified}
        amount={amount}
        locale={locale}
      />
    </StatStrip>
  );
}

function QualifiedCard({
  qualified,
  amount,
  locale,
}: Readonly<{
  qualified?: Qualified;
  amount: (value: number | undefined) => string;
  locale: Locale;
}>) {
  const t = useT();
  if (!qualified || qualified.status === "not_enforced") {
    return (
      <StatCard
        label={t("reporting.compare.coverageQualified")}
        value={t("reporting.compare.qualifiedOff")}
        detail={t("reporting.compare.qualifiedOffBasis")}
      />
    );
  }
  return (
    <StatCard
      label={t("reporting.compare.coverageQualified")}
      value={coverageValue(
        qualified.coverage,
        qualified.status === "target_reached",
        t,
        locale,
      )}
      detail={t("reporting.compare.qualifiedBasis", {
        open: amount(qualified.open_pipeline),
      })}
    />
  );
}

function gapWords(gap: number) {
  if (gap > 0) return "reporting.compare.short";
  return gap < 0 ? "reporting.compare.ahead" : "reporting.compare.atTarget";
}

function coverageValue(
  coverage: number | undefined,
  reached: boolean,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  if (reached || coverage == null) return t("reporting.compare.reached");
  return t("reporting.compare.ratio", {
    ratio: formatDecimal(coverage, locale, 1),
  });
}

type StateKey =
  | "no_target"
  | "fx_missing"
  | "partial_visibility"
  | "colleague_withheld"
  | "call_below_won"
  | `incompatible.${NonNullable<Comparison["incompatibility"]>}`;

function stateKey(comparison: Comparison): StateKey | null {
  switch (comparison.status) {
    case "incompatible":
      return `incompatible.${comparison.incompatibility ?? "period"}`;
    case "no_target":
    case "fx_missing":
    case "partial_visibility":
    case "colleague_withheld":
    case "call_below_won":
      return comparison.status;
    default:
      return null;
  }
}

// Why the quarter has no comparison, and what would give it one.
function ComparisonState({
  comparison,
  onApply,
}: Readonly<{
  comparison: Comparison;
  onApply?: (selection: ReportingSelection) => void;
}>) {
  const t = useT();
  const key = stateKey(comparison);
  if (!key) return null;
  const supported = comparison.supported_selection;
  const why = comparison.incompatibility;
  const apply =
    supported && onApply && why && why !== "currency" ? (
      <Button onClick={() => onApply(supported)}>
        {t(`reporting.compare.apply.${why}`)}
      </Button>
    ) : undefined;
  return (
    <Callout
      tone={comparison.status === "no_target" ? "info" : "warning"}
      kind="standing"
      title={t(`reporting.compare.${key}`)}
      actions={apply}
    >
      <p>{t(`reporting.compare.${key}Body`)}</p>
    </Callout>
  );
}
