// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { Meter } from "../../design-system/readings";
import { useLocale, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCan, useCanWrite } from "../../lib/capability";
import { targetCard } from "./analytics.outcomes.targets";
import { scopeDials } from "./analytics.overview.context";
import {
  type ReportingEvaluation,
  reportingPeriodLabel,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";

// This month's targets as a strip of their own under the week's figure. They
// name their own month, so a week is never read against a month's target.
// `ownScope` is the reader's own records, which the target form opens on.
export function MonthlyTargets({
  evaluation,
  ownScope,
}: Readonly<{ evaluation: ReportingEvaluation; ownScope: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const metricName = useMetricNames();
  const readsTargets = useCan("sales_target", "read");
  const canSet = useCanWrite("sales_target", "create");
  const { context } = evaluation;
  if (!readsTargets || !context.target_interval) return null;
  const month = reportingPeriodLabel(
    context.target_interval,
    context.timezone,
    locale,
  );
  const held = evaluation.metrics.filter(
    (metric) => metric.target != null && !metric.coverage.withheld,
  );
  return (
    <div className="analytics-hero-aside">
      <p className="t-label">
        {t("analytics.overview.monthlyTitle", { month })}
      </p>
      {held.length === 0 ? (
        <p className="t-caption">
          {t("analytics.overview.noMonthlyTarget")}.
          {canSet && (
            <>
              {" "}
              <AppLink
                href={analyticsSectionHref("targets", scopeDials(ownScope))}
              >
                {t("analytics.overview.setTarget")}
              </AppLink>
            </>
          )}
        </p>
      ) : (
        held.map((metric) => {
          const card = targetCard(
            metric,
            context,
            metricName(metric.id),
            t,
            locale,
          );
          return (
            <div key={card.key}>
              {card.meter && (
                <Meter
                  value={card.meter.filled}
                  max={card.meter.total}
                  label={card.label}
                  dense
                />
              )}
              <p className="t-caption">
                {card.label}: {card.value}
              </p>
            </div>
          );
        })
      )}
    </div>
  );
}
