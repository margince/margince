import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, SegmentedControl, StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { EvidenceReceipt } from "../../design-system/evidencereceipt";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SegmentBar } from "../../design-system/readings";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatDateAbbrev,
  formatDateTime,
  formatDayRange,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
} from "../../format/format";
import { formatMoneyOrWord } from "../../format/moneyword";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { type AnalyticsSelection, writableScope } from "./analytics.context";
import { ForecastCallEditor } from "./analytics.forecast.calleditor";
import { ForecastDeals, openForecastDeals } from "./analytics.forecast.deals";
import {
  LandingCard,
  ModelRangeCard,
  RecordsLandingCard,
  rangeOf,
  SufficiencyCard,
} from "./analytics.forecast.landing";
import { QualifiedAnswer } from "./analytics.forecast.qualified";
import { ForecastReview } from "./analytics.forecast.review";
import { PeriodCaption } from "./analytics.period";
import {
  FORECAST_PERIODS,
  type ForecastPeriod,
  useForecastReadings,
} from "./forecast.queries";
import { CallEditorProvider } from "./inputcheck.call";
import { ReportingForecastGraphs } from "./reporting.forecast";
import "./analytics.css";

type Readings = components["schemas"]["ForecastReadings"];

export function ForecastView({
  selection,
  canSubmit,
  scopeControl,
  emptyNotice,
}: Readonly<{
  selection: AnalyticsSelection;
  canSubmit: boolean;
  scopeControl?: ReactNode;
  emptyNotice?: (holdsDeals: boolean | undefined) => ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [period, setPeriod] = useState<ForecastPeriod>("quarter");
  // Whether the call editor is open. Held here rather than in the editor,
  // because the verb that opens it stands in the toolbar beside the period it
  // files against, outside the read the editor needs.
  const [editing, setEditing] = useState(false);
  // Only a reader who may call, calling for ONE population: a blended view of
  // several teams has no single call to record.
  const canCall = canSubmit && writableScope(selection.scope) != null;

  // The same read the morning's pipeline counter makes, through the same key:
  // two surfaces asking what the pipeline is worth must not get two answers.
  const readings = useForecastReadings(selection.scope, period);

  return (
    <div className="analytics-stack">
      {/* Which window the figures below are over, and the verb that records a
          call about it. Above the readings rather than beside them, because
          every number on this page changes when the window moves — a control
          nested among them would read as filtering one. */}
      <FilterBar
        label={t("analytics.filters")}
        actions={
          canCall && !editing ? (
            <Button onClick={() => setEditing(true)}>
              {t("forecast.updateCall")}
            </Button>
          ) : null
        }
        caption={readings.data && forecastPeriod(readings.data, locale, t)}
      >
        {scopeControl}
        <SegmentedControl
          label={t("forecast.period")}
          options={FORECAST_PERIODS}
          value={period}
          onChange={setPeriod}
          labels={{
            quarter: t("forecast.period.quarter"),
            month: t("forecast.period.month"),
            week: t("forecast.period.week"),
          }}
        />
      </FilterBar>
      <QueryGate query={readings} pendingLabel={t("forecast.updateCall")}>
        {(data) => (
          <>
            {/* Directly under the verb that opened it, above the figures it
                answers, so the form appears where the reader's eye already is. */}
            {canCall && editing ? (
              <ForecastCallEditor
                readings={data}
                selection={selection}
                period={period}
                onClose={() => setEditing(false)}
              />
            ) : null}
            {emptyNotice?.(data.scope_holds_deals)}
            <ForecastAnswer
              readings={data}
              locale={locale}
              period={period}
              onOpenDeals={openForecastDeals}
            />
            <ForecastDeals scope={selection.scope} period={period} />
            {period === "quarter" && (
              <ReportingForecastGraphs scope={selection.scope} />
            )}
            {/* What to check comes first: a manager with ten minutes reads what
                needs doing, and reads the receipt beside it when a number looks
                wrong. */}
            <div className="analytics-pair">
              <CallEditorProvider
                editor={
                  canCall
                    ? {
                        open: () => setEditing(true),
                        scope: writableScope(selection.scope) ?? {
                          scope_kind: "",
                        },
                        periodStart: data.period_start ?? "",
                      }
                    : null
                }
              >
                <ForecastReview />
              </CallEditorProvider>
              <EvidenceReceipt
                title={t("forecast.receipt")}
                counts={[
                  {
                    key: "eligible",
                    term: t("forecast.eligible"),
                    value: formatNumber(data.eligible_count, locale),
                  },
                  {
                    key: "priced",
                    term: t("forecast.priced"),
                    value: formatNumber(data.priced_count, locale),
                  },
                  {
                    key: "confirmed",
                    term: t("forecast.confirmed"),
                    value: formatNumber(data.confirmed_date_count, locale),
                  },
                  {
                    key: "fx",
                    term: t("forecast.fxMissing"),
                    value: formatNumber(data.fx_missing_count, locale),
                  },
                ]}
              />
            </div>
          </>
        )}
      </QueryGate>
    </div>
  );
}

// The forecast's whole period and the instant its actuals were read at, the
// two facts the Overview's caption states too.
function forecastPeriod(
  readings: Readings,
  locale: Locale,
  t: Translator,
): ReactNode {
  const zone = readings.timezone;
  return (
    <PeriodCaption
      period={
        readings.period_start && readings.period_end
          ? formatDayRange(
              readings.period_start,
              readings.period_end,
              locale,
              zone,
            )
          : undefined
      }
      through={t("reporting.actualsThrough", {
        at: formatDateTime(readings.as_of, locale, zone),
      })}
    />
  );
}

// Compare the period forecast with won sales plus confirmed committed deals.
function callDetail(
  call: NonNullable<Readings["current_call"]>,
  readings: Readings,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  // The day the call was authored, in the period's own zone: a figure and the
  // date beside it are bucketed on one calendar.
  const date = formatDateAbbrev(call.created_at, locale, readings.timezone);
  const difference =
    call.amount_minor - (readings.won_minor + readings.evidence_minor);
  if (difference === 0) {
    return t("forecast.currentCallDetailEven", { date });
  }
  const gap = formatMoneyOrWord(
    Math.abs(difference),
    readings.base_currency,
    locale,
    t("format.notForecast"),
    formatMoneyCompact,
  );
  return t(
    difference > 0
      ? "forecast.currentCallDetailOver"
      : "forecast.currentCallDetailUnder",
    { date, gap },
  );
}

// The answer, in one sentence and then in three readings. A shared view has no
// deals panel, so it passes no `onOpenDeals`.
function ForecastAnswer({
  readings,
  locale,
  period,
  landingUnrecorded = false,
  onOpenDeals,
}: Readonly<{
  readings: Readings;
  locale: Locale;
  period: ForecastPeriod;
  // A snapshot frozen before its landing was kept: the basis is unknown.
  landingUnrecorded?: boolean;
  onOpenDeals?: () => void;
}>) {
  const t = useT();
  const currency = readings.base_currency;
  // The sentence carries the amount in FULL — it is read once, at prose width,
  // and a call somebody authored to the cent is a number they should meet as
  // they wrote it. The slots below carry the same figures compactly, because a
  // slot is a hundred points wide and a full amount clips there.
  const money = (minor: number | null | undefined) =>
    formatMoneyOrAbsent(minor ?? null, currency, locale);
  // Compact, and a WORD where the pair cannot be said as money at all: a slot
  // is about a hundred points wide, and one compared across a row must not
  // answer with a glyph. The landing cards at the end of this strip answer the
  // same way, so the row reads as one comparison.
  const slot = (minor: number | null | undefined) =>
    formatMoneyOrWord(
      minor,
      currency,
      locale,
      t("format.notForecast"),
      formatMoneyCompact,
    );
  const call = readings.current_call;
  const range = rangeOf(readings.range_low_minor, readings.range_high_minor);
  // What best case adds on top of the evidence it contains. Never below zero:
  // a superset smaller than its subset is a server fault, shown as nothing added.
  const bestCaseAdds = Math.max(
    0,
    readings.best_case_minor - readings.evidence_minor,
  );

  return (
    <>
      {/* The screen's own answer, so it is the screen's own section: a Callout
          says something ABOUT a surface, and this IS the surface's content —
          the question as the panel's title and the answer as its body. */}
      <Panel title={t("forecast.question")}>
        <PanelBody>
          {/* The call and the supported figure, and the gap between them. A
              reader shown only one of the two has no way to tell whether the
              call is ahead of the evidence or behind it. */}
          <p>
            {call
              ? t("forecast.answerWithCall", {
                  call: money(call.amount_minor),
                  evidence: money(readings.won_minor + readings.evidence_minor),
                })
              : t("forecast.answerNoCall", {
                  evidence: money(readings.won_minor + readings.evidence_minor),
                })}
          </p>
          {/* The same answer as one shape: what is banked, what is committed
              with a date, what best case adds, and the call across them. The
              three are DISJOINT on the server — evidence and best case count
              open deals only, and best case contains evidence — so they are
              laid end to end, and best case contributes only what it adds. */}
          <SegmentBar
            label={t("forecast.makeup")}
            parts={[
              {
                key: "won",
                label: t("forecast.alreadyWon"),
                value: readings.won_minor,
                amount: slot(readings.won_minor),
              },
              {
                key: "evidence",
                label: t("forecast.evidence"),
                value: readings.evidence_minor,
                amount: slot(readings.evidence_minor),
              },
              {
                key: "bestCase",
                label: t("forecast.bestCaseAdds"),
                value: bestCaseAdds,
                amount: slot(bestCaseAdds),
              },
            ]}
            marker={
              call
                ? {
                    key: "call",
                    label: t("forecast.currentCall"),
                    value: call.amount_minor,
                    amount: slot(call.amount_minor),
                  }
                : undefined
            }
          />
          {call && (
            <p className="t-caption">
              {callDetail(call, readings, locale, t)}
              {call.baseline === "unavailable" &&
                ` ${t("forecast.callHistoryUnavailable")}`}
            </p>
          )}
          <QualifiedAnswer readings={readings} money={money} />
        </PanelBody>
      </Panel>

      {/* An unpriced deal is a real deal contributing zero money, so the gap
          between eligible and priced is stated beside the total rather than
          left in the receipt alone. */}
      {readings.priced_count < readings.eligible_count && (
        <Callout
          tone="warning"
          kind="standing"
          title={t("forecast.partialTitle")}
        >
          {t("forecast.partial", {
            priced: formatNumber(readings.priced_count, locale),
            eligible: formatNumber(readings.eligible_count, locale),
          })}
        </Callout>
      )}

      {/* EVERY slot declares the narrow shape, the landing pair included: the
          fold is the strip's, so a card that did not declare it would keep its
          box while the rows beside it lost theirs.

          Both are absent for a managed-teams reading, which covers several
          populations at once: a landing summed across books that are called
          separately, and a coverage rate blended over them, would each
          describe none of them. With neither, no strip: an empty one still
          takes its row and its gaps. A share has a range without a landing
          when its snapshot predates the landing, so the range alone keeps
          the strip. */}
      {(readings.landing ||
        readings.sufficiency ||
        range ||
        landingUnrecorded) && (
        <StatStrip>
          {readings.landing ? (
            <LandingCard
              landing={readings.landing}
              currency={currency}
              locale={locale}
              callMinor={
                readings.landing.measure === "manager_call"
                  ? undefined
                  : readings.current_call?.amount_minor
              }
            />
          ) : (
            landingUnrecorded && (
              <StatCard
                narrow="row"
                label={t("forecast.landing")}
                value={t("forecast.landingUnrecorded")}
                detail={t("forecast.basis.unknown")}
              />
            )
          )}
          {(readings.landing || range) && (
            <ModelRangeCard
              label={t(`forecast.model.${period}`)}
              range={range}
              wonMinor={readings.won_minor}
              unpriced={readings.eligible_count - readings.priced_count}
              fxMissing={readings.fx_missing_count}
              money={slot}
              onOpen={onOpenDeals}
            />
          )}
          {readings.landing?.system_minor != null && (
            <RecordsLandingCard
              systemMinor={readings.landing.system_minor}
              currency={currency}
              locale={locale}
            />
          )}
          {readings.sufficiency && (
            <SufficiencyCard
              sufficiency={readings.sufficiency}
              currency={currency}
              locale={locale}
            />
          )}
        </StatStrip>
      )}
    </>
  );
}

export function SharedForecastView({ token }: Readonly<{ token: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const query = useQuery({
    queryKey: ["shared-forecast", token],
    queryFn: async () => {
      return unwrap(
        await api.GET("/forecast/shared/{token}", {
          params: { path: { token } },
        }),
      );
    },
  });
  return (
    <div className="wrap">
      <QueryGate query={query} pendingLabel={t("analytics.sectionForecast")}>
        {(view) => (
          <>
            <p className="t-caption">
              {view.kind === "snapshot"
                ? t("reporting.frozen")
                : t("reporting.live")}
              {view.as_of
                ? ` · ${formatDateTime(view.as_of, locale, zone)}`
                : ""}
            </p>
            {view.withheld && (
              <Callout tone="warning" title={t("reporting.restricted")}>
                {t("reporting.restricted")}
              </Callout>
            )}
            {/* A share always reads the current quarter. */}
            <ForecastAnswer
              readings={view.readings}
              locale={locale}
              period="quarter"
              landingUnrecorded={
                view.kind === "snapshot" && !view.readings.landing
              }
            />
          </>
        )}
      </QueryGate>
    </div>
  );
}
