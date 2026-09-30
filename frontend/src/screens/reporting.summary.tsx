import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  metricLabel,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingCompactAmount,
  reportingPeriodLabel,
} from "./reporting.model";

export function TargetProgress({
  value,
  target,
  format,
}: Readonly<{
  value: number | null;
  target?: number | null;
  format: (value: number) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (target == null || value == null) return null;
  return (
    <p className="t-caption">
      {t("reporting.remaining")}: {format(Math.max(0, target - value))}
      {target > 0
        ? ` · ${t("reporting.attainment", { percent: formatNumber(Math.round((value / target) * 100), locale) })}`
        : ""}
    </p>
  );
}

export function ResultsSummary({
  evaluation,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const metrics = evaluation.metrics.filter((metric) =>
    ["bookings_won", "meetings_held", "accepted_opportunities"].includes(
      metric.id,
    ),
  );
  if (!metrics.length) return null;
  return (
    <StatStrip>
      {metrics.map((metric) => {
        const amount = (value: number | null | undefined) =>
          reportingCompactAmount(
            value,
            metric.unit,
            evaluation.context.currency,
            locale,
          );
        const actual = metric.target_actual;
        const target = metric.target;
        const comparable = target != null && target > 0 && actual != null;
        return (
          <StatCard
            key={metric.id}
            narrow="row"
            label={metricLabel(metric.id, t)}
            value={amount(metric.value)}
            detail={
              <>
                <span>
                  {reportingPeriodLabel(
                    evaluation.context.interval,
                    evaluation.context.timezone,
                    locale,
                  )}
                </span>
                {comparable && (
                  <span>
                    {t("reporting.targetPeriodSummary", {
                      actual: amount(actual),
                      target: amount(target),
                      percent: formatNumber(
                        Math.round((actual / target) * 100),
                        locale,
                      ),
                    })}{" "}
                    ·{" "}
                    {t(
                      actual <= target
                        ? "reporting.targetRemaining"
                        : "reporting.targetExceeded",
                      { amount: amount(Math.abs(target - actual)) },
                    )}
                    {evaluation.context.target_interval &&
                      ` · ${reportingPeriodLabel(evaluation.context.target_interval, evaluation.context.timezone, locale)}`}
                  </span>
                )}
                {metric.coverage.status !== "ok" && (
                  <span>{t(`reporting.status.${metric.coverage.status}`)}</span>
                )}
              </>
            }
            basis={
              <>
                <p>
                  {reportingAmount(
                    metric.value,
                    metric.unit,
                    evaluation.context.currency,
                    locale,
                  )}
                </p>
                {comparable && (
                  <p>
                    {t(
                      actual <= target
                        ? "reporting.targetRemaining"
                        : "reporting.targetExceeded",
                      { amount: amount(Math.abs(target - actual)) },
                    )}
                  </p>
                )}
                <p>
                  {metric.coverage.reason ??
                    t(`reporting.status.${metric.coverage.status}`)}
                </p>
              </>
            }
            onOpen={
              metric.coverage.status === "no_data"
                ? undefined
                : () => onEvidence(metric.evidence)
            }
            meter={comparable ? { filled: actual, total: target } : undefined}
          />
        );
      })}
    </StatStrip>
  );
}
