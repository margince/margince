// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The open deals behind the weighted figure, each with the chance it is
// weighted by and why. Its own read, so the headline does not wait for it.

import { Badge, EmptyState } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Disclosure } from "../../design-system/disclosure";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatMoneyOrAbsent, formatNumber } from "../../format/format";
import {
  type Locale,
  type PluralTranslator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { usePipelines } from "../../lib/pipelines.queries";
import type { AnalyticsScope } from "./analytics.context";
import {
  type ForecastContribution,
  type ForecastPeriod,
  useForecastContributions,
} from "./forecast.queries";
import { signalSentence } from "./forecast.signals";

type T = ReturnType<typeof useT>;

const percent = (value: number, locale: Locale) =>
  `${formatNumber(value, locale)}%`;

/** One line that says how a deal's weight was made. */
export function chanceExplanation(
  row: ForecastContribution,
  t: T,
  plural: PluralTranslator,
  locale: Locale,
): string {
  const parts = [
    t("forecast.deals.typed", {
      percent: percent(row.typed_probability ?? row.chance_in_use, locale),
    }),
  ];
  if (row.evidence_n !== undefined) {
    parts.push(
      plural("forecast.deals.recorded", row.evidence_n, {
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

type Words = Readonly<{
  t: T;
  plural: PluralTranslator;
  locale: Locale;
  stageName: (stageId: string) => string | undefined;
}>;

function dealColumns(
  words: Words,
  currency: string,
): DataTableColumn<ForecastContribution>[] {
  const { t, plural, locale, stageName } = words;
  return [
    {
      key: "deal",
      header: t("forecast.deals.colDeal"),
      grow: true,
      render: (row) => (
        <CellStack>
          <span>{row.deal_name}</span>
          <span className="t-caption">
            {chanceExplanation(row, t, plural, locale)}
          </span>
        </CellStack>
      ),
    },
    {
      key: "signals",
      header: t("forecast.deals.colSignals"),
      render: (row) =>
        row.signals.map((signal) => (
          <Badge key={signal.code} tone="warning">
            {signalSentence(signal, t, plural, stageName, (value) =>
              formatNumber(value, locale),
            )}
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
  const plural = usePlural();
  const { locale } = useLocale();
  const contributions = useForecastContributions(scope, period);
  const pipelines = usePipelines();
  const stages = new Map(
    (pipelines.data ?? []).flatMap((pipeline) =>
      (pipeline.stages ?? []).map((stage) => [stage.id, stage.name] as const),
    ),
  );
  const words: Words = {
    t,
    plural,
    locale,
    stageName: (stageId) => stages.get(stageId),
  };
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
                  {data.contributions_limited && (
                    <Callout
                      tone="info"
                      kind="standing"
                      title={t("forecast.deals.limitedTitle")}
                    >
                      {t("forecast.deals.limited")}
                    </Callout>
                  )}
                  {open.length === 0 ? (
                    <EmptyState>{t("forecast.deals.none")}</EmptyState>
                  ) : (
                    <DataTable<ForecastContribution>
                      label={label}
                      rows={open}
                      rowKey={(row) => row.deal_id}
                      columns={dealColumns(words, data.base_currency)}
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
