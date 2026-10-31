// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The open deals behind the weighted figure, each with the chance it is
// weighted by and why. Its own read, so the headline does not wait for it.

import { Badge, EmptyState } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Disclosure } from "../../design-system/disclosure";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatMoneyOrAbsent, formatNumber } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import type { AnalyticsScope } from "./analytics.context";
import {
  type ForecastContribution,
  type ForecastPeriod,
  useForecastContributions,
} from "./forecast.queries";

type T = ReturnType<typeof useT>;

const percent = (value: number, locale: Locale) =>
  `${formatNumber(value, locale)}%`;

/** One line that says how a deal's weight was made. */
export function chanceExplanation(
  row: ForecastContribution,
  t: T,
  locale: Locale,
): string {
  const parts = [
    t("forecast.deals.typed", {
      percent: percent(row.typed_probability ?? row.chance_in_use, locale),
    }),
  ];
  if (row.evidence_n !== undefined) {
    parts.push(
      t("forecast.deals.recorded", {
        count: formatNumber(row.evidence_n, locale),
      }),
    );
  }
  parts.push(
    t("forecast.deals.uses", { percent: percent(row.chance_in_use, locale) }),
  );
  if (row.applied_probability !== row.chance_in_use) {
    parts.push(
      t("forecast.deals.afterRisk", {
        percent: percent(row.applied_probability, locale),
      }),
    );
  }
  return parts.join(" · ");
}

function dealColumns(
  t: T,
  locale: Locale,
  currency: string,
): DataTableColumn<ForecastContribution>[] {
  return [
    {
      key: "deal",
      header: t("forecast.deals.colDeal"),
      grow: true,
      render: (row) => (
        <CellStack>
          <span>{row.deal_name}</span>
          <span className="t-caption">{chanceExplanation(row, t, locale)}</span>
        </CellStack>
      ),
    },
    {
      key: "signals",
      header: t("forecast.deals.colSignals"),
      render: (row) =>
        row.signals.map((signal) => (
          <Badge key={signal} tone="warning">
            {t(`forecast.signal.${signal}`)}
          </Badge>
        )),
    },
    {
      key: "amount",
      header: t("forecast.deals.colAmount"),
      align: "end",
      render: (row) =>
        formatMoneyOrAbsent(row.base_minor ?? null, currency, locale),
    },
    {
      key: "weighted",
      header: t("forecast.deals.colWeighted"),
      align: "end",
      render: (row) =>
        formatMoneyOrAbsent(row.weighted_minor, currency, locale),
    },
  ];
}

/** The per-deal drilldown under the forecast's answer. */
export function ForecastDeals({
  scope,
  period,
}: Readonly<{ scope: AnalyticsScope | undefined; period: ForecastPeriod }>) {
  const t = useT();
  const { locale } = useLocale();
  const contributions = useForecastContributions(scope, period);
  const label = t("forecast.deals.title");
  return (
    <Panel title={label}>
      <PanelBody>
        <Disclosure summary={t("forecast.deals.open")}>
          <QueryGate query={contributions} pendingLabel={label}>
            {(data) => {
              const open = (data.contributions ?? []).filter(
                (row) => row.in_open,
              );
              return (
                <>
                  <PanelIntro>
                    {t(
                      data.self_correcting
                        ? "forecast.deals.introOn"
                        : "forecast.deals.introOff",
                    )}
                  </PanelIntro>
                  {open.length === 0 ? (
                    <EmptyState>{t("forecast.deals.none")}</EmptyState>
                  ) : (
                    <DataTable<ForecastContribution>
                      label={label}
                      rows={open}
                      rowKey={(row) => row.deal_id}
                      columns={dealColumns(t, locale, data.base_currency)}
                    />
                  )}
                </>
              );
            }}
          </QueryGate>
        </Disclosure>
      </PanelBody>
    </Panel>
  );
}
