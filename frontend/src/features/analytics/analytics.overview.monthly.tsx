// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { StatStrip } from "../../design-system/statstrip";
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

// This month's targets, named by their own month so a week's figure is never
// read against a month's target.
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
    <Panel title={t("analytics.overview.monthlyTitle", { month })}>
      <PanelBody>
        {held.length === 0 ? (
          <Callout
            tone="info"
            kind="standing"
            title={t("analytics.overview.noMonthlyTarget")}
            actions={
              canSet ? (
                <AppLink
                  className="btn btn-ghost"
                  href={analyticsSectionHref("targets", scopeDials(ownScope))}
                >
                  {t("analytics.overview.setTarget")}
                </AppLink>
              ) : undefined
            }
          >
            <p>
              {t(
                canSet
                  ? "analytics.overview.setMonthlyTargetBody"
                  : "analytics.overview.targetWho",
              )}
            </p>
          </Callout>
        ) : (
          <StatStrip label={t("analytics.overview.monthlyTitle", { month })}>
            {held.map((metric) => {
              const card = targetCard(
                metric,
                context,
                metricName(metric.id),
                t,
                locale,
              );
              return (
                <StatCard
                  key={card.key}
                  narrow="row"
                  label={card.label}
                  value={card.value}
                  detail={card.detail}
                  meter={card.meter}
                />
              );
            })}
          </StatStrip>
        )}
      </PanelBody>
    </Panel>
  );
}
