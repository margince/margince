// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The weighted figure and the open deals behind it, each with the chance it is
// weighted by and why. Its own read, so the headline does not wait for it.

import { Badge, EmptyState } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Disclosure } from "../../design-system/disclosure";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import {
  formatAnalyticsMoneyOrAbsent,
  formatAnalyticsNumber,
  formatAnalyticsPercentPoints,
} from "../../format/format";
import {
  type Locale,
  type PluralTranslator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { signalSentence } from "../../lib/forecastsignals";
import { valueFor } from "../../lib/labelfor";
import { stageNames, usePipelines } from "../../lib/pipelines.queries";
import type { AnalyticsScope } from "./analytics.context";
import {
  type ForecastContribution,
  type ForecastPeriod,
  type ForecastReadings,
  useForecastContributions,
} from "./forecast.queries";

type T = ReturnType<typeof useT>;

/** One line that says how a deal's weight was made. */
export function chanceExplanation(
  row: ForecastContribution,
  t: T,
  plural: PluralTranslator,
  locale: Locale,
): string {
  const parts = [
    t("forecast.deals.typed", {
      percent: formatAnalyticsPercentPoints(
        row.typed_probability ?? row.chance_in_use,
        locale,
      ),
    }),
  ];
  // Evidence stops at the start of today, so a deal closed today counts tomorrow.
  if (row.evidence_n === 0) {
    parts.push(t("forecast.deals.recordedNone"));
  } else if (row.evidence_n !== undefined) {
    parts.push(
      plural("forecast.deals.recorded", row.evidence_n, {
        count: formatAnalyticsNumber(row.evidence_n, locale),
      }),
    );
  }
  parts.push(
    t("forecast.deals.uses", {
      percent: formatAnalyticsPercentPoints(row.chance_in_use, locale),
    }),
  );
  if (row.applied_probability !== row.chance_in_use) {
    parts.push(
      t("forecast.deals.afterRisk", {
        percent: formatAnalyticsPercentPoints(row.applied_probability, locale),
      }),
    );
  }
  return parts.join(" · ");
}

const EXCLUSION_WORD: Readonly<
  Record<NonNullable<ForecastContribution["exclusion_reason"]>, MessageKey>
> = {
  unpriced: "analytics.forecastNoAmount",
  fx_missing: "forecast.fxMissing",
};

// A masked deal, or a reason this build does not know, keeps the absent figure.
function exclusionWord(row: ForecastContribution): MessageKey | undefined {
  return row.exclusion_reason
    ? valueFor(EXCLUSION_WORD, row.exclusion_reason, undefined)
    : undefined;
}

function amountCell(
  row: ForecastContribution,
  currency: string,
  { t, locale }: Words,
): string {
  const word = exclusionWord(row);
  if (word) return t(word);
  return formatAnalyticsMoneyOrAbsent(row.base_minor ?? null, currency, locale);
}

// An excluded deal adds nothing, which is not the same as adding zero.
function weightedCell(
  row: ForecastContribution,
  currency: string,
  locale: Locale,
): string {
  return formatAnalyticsMoneyOrAbsent(
    row.exclusion_reason ? null : row.weighted_minor,
    currency,
    locale,
  );
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
      fold: "title",
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
              formatAnalyticsNumber(value, locale),
            )}
          </Badge>
        )),
    },
    {
      key: "amount",
      header: t("forecast.deals.colAmount"),
      align: "end",
      render: (row) => amountCell(row, currency, words),
      // The reason reads on its own, without the column name before it.
      folded: (row) =>
        exclusionWord(row)
          ? undefined
          : t("forecast.deals.foldedAmount", {
              amount: amountCell(row, currency, words),
            }),
    },
    {
      key: "weighted",
      header: t("forecast.deals.colWeighted"),
      align: "end",
      fold: "end",
      render: (row) => weightedCell(row, currency, locale),
      folded: (row) =>
        t("forecast.deals.foldedWeighted", {
          amount: weightedCell(row, currency, locale),
        }),
    },
  ];
}

// The panel's place on the page, so a reading drawn from these deals can lead to them.
export const FORECAST_DEALS_ANCHOR = "forecast-deals";

/** Brings the deals panel into view and moves keyboard focus onto it. */
export function openForecastDeals(): void {
  const panel = document.getElementById(FORECAST_DEALS_ANCHOR);
  panel?.scrollIntoView?.({ block: "start" });
  panel?.focus();
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
  const stages = stageNames(pipelines.data ?? []);
  const words: Words = {
    t,
    plural,
    locale,
    stageName: (stageId) => stages.get(stageId),
  };
  const label = t("forecast.deals.title");
  return (
    // Focusable only by script, so the jump lands keyboard focus where the eye goes.
    <div id={FORECAST_DEALS_ANCHOR} tabIndex={-1}>
      <Panel title={label}>
        <PanelBody>
          <QueryGate query={contributions} pendingLabel={label}>
            {(data) => {
              const open = (data.contributions ?? []).filter(
                (row) => row.in_open,
              );
              return (
                <>
                  <PanelIntro>
                    {weightedTotal(data, open.length, words)}
                  </PanelIntro>
                  {data.contributions_limited && (
                    <Callout
                      tone="info"
                      kind="standing"
                      title={t("forecast.deals.limitedTitle")}
                    >
                      {data.contributions_limited_reason}
                    </Callout>
                  )}
                  <Disclosure summary={t("forecast.deals.open")}>
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
                        fold
                        label={label}
                        rows={open}
                        rowKey={(row) => row.deal_id}
                        columns={dealColumns(words, data.base_currency)}
                      />
                    )}
                  </Disclosure>
                </>
              );
            }}
          </QueryGate>
        </PanelBody>
      </Panel>
    </div>
  );
}

// The figure the panel is named for. While the rows are the reader's own, their
// count is not the total's, so the sentence names no count then.
function weightedTotal(
  data: ForecastReadings,
  openCount: number,
  { t, plural, locale }: Words,
): string {
  const amount = formatAnalyticsMoneyOrAbsent(
    data.weighted_minor,
    data.base_currency,
    locale,
  );
  if (data.contributions_limited) {
    return t("forecast.deals.totalLimited", { amount });
  }
  return plural("forecast.deals.total", openCount, {
    amount,
    count: formatAnalyticsNumber(openCount, locale),
  });
}
