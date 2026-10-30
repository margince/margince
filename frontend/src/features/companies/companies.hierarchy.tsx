import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EmptyState, Skeleton } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { Stack } from "../../design-system/stack";
import { formatDateTime, formatMoney, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf, throwProblem } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import {
  type RollupPeriod,
  RollupPeriodControl,
  rollupQuery,
} from "./companies.hierarchy.period";
import "./company360.css";

type CompanyHierarchyRollup = components["schemas"]["CompanyHierarchyRollup"];

// A missing stored FX rate fails the whole rollup read with 422
// fx_rate_unavailable (never a rate of 1, never zeros). This marker lets the
// render branch on that one cause without parsing the problem body again.
class FxUnavailableError extends Error {}

async function fetchHierarchyRollup(
  companyId: string,
  period: RollupPeriod | null,
): Promise<CompanyHierarchyRollup> {
  const { data, error } = await api.GET("/companies/{id}/hierarchy-rollup", {
    params: { path: { id: companyId }, query: rollupQuery(period) },
  });
  if (error) {
    if (error.code === "fx_rate_unavailable") {
      throw new FxUnavailableError();
    }
    throwProblem(error);
  }
  return data;
}

// The company hierarchy roll-up, read-only. Money renders only when both
// amount_minor and currency are present (Money's fields are individually
// optional on the wire), never a hand-formatted or zero-filled figure. The
// period moves the won sales alone; the pipeline is today's.
export function HierarchyRollupPanel({
  companyId,
}: Readonly<{ companyId: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [period, setPeriod] = useState<RollupPeriod | null>(null);
  const rollupQuery = useQuery({
    queryKey: ["rollup", companyId, period],
    queryFn: () => fetchHierarchyRollup(companyId, period),
    placeholderData: keepPreviousData,
  });

  if (rollupQuery.isPending) {
    return (
      <Stack gap="3">
        <Skeleton width="60%" />
        <Skeleton width="90%" />
        <Skeleton width="75%" />
      </Stack>
    );
  }
  if (rollupQuery.isError) {
    if (rollupQuery.error instanceof FxUnavailableError) {
      return <EmptyState>{t("rollup.fxUnavailable")}</EmptyState>;
    }
    return <EmptyState>{problemMessageOf(rollupQuery.error, t)}</EmptyState>;
  }

  const rollup = rollupQuery.data;
  const won = rollup.closed_won_period;
  const money = (value: CompanyHierarchyRollup["weighted_pipeline"]) =>
    value.amount_minor != null && value.currency
      ? formatMoney(value.amount_minor, value.currency, locale)
      : "—";

  return (
    <Panel
      title={t("tab.rollup")}
      actions={
        <RollupPeriodControl
          rollup={rollup}
          period={period}
          onChange={setPeriod}
        />
      }
    >
      <PanelBody>
        <dl className="firmo">
          <div>
            <dt className="t-eyebrow">{t("rollup.weightedPipeline")}</dt>
            <dd className="t-num">{money(rollup.weighted_pipeline)}</dd>
          </div>
          <div>
            <dt className="t-eyebrow">
              {won.chosen
                ? t("rollup.wonIn", { period: won.label })
                : t("rollup.closedWon")}
            </dt>
            <dd className="t-num">{money(rollup.closed_won)}</dd>
          </div>
          <div>
            <dt className="t-eyebrow">{t("rollup.activity30d")}</dt>
            <dd>{formatNumber(rollup.activity_count_30d, locale)}</dd>
          </div>
          <div>
            <dt className="t-eyebrow">{t("rollup.accounts")}</dt>
            <dd>{formatNumber(rollup.aggregated_account_count, locale)}</dd>
          </div>
        </dl>
        {rollup.restricted_excluded.length > 0 && (
          <p className="t-caption co-rollup-note">
            {plural("rollup.excluded", rollup.restricted_excluded.length, {
              count: formatNumber(rollup.restricted_excluded.length, locale),
            })}
          </p>
        )}
        <p className="t-caption co-rollup-note">
          {t("rollup.periodNote", { period: won.label })}
        </p>
        <p className="t-caption co-rollup-note">
          {t("rollup.computedAt", {
            when: formatDateTime(rollup.computed_at, locale, recordZone),
          })}
        </p>
      </PanelBody>
    </Panel>
  );
}
