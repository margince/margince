import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  PendingBody,
  SegmentedControl,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { EvidenceReceipt } from "../../design-system/evidencereceipt";
import { FilterBar } from "../../design-system/filterbar";
import { QueryGate } from "../../design-system/querystates";
import {
  formatDateTime,
  formatDayRange,
  formatNumber,
} from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { usePipelines } from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import {
  currentParams,
  replaceDial,
  replaceParams,
  useUrlParams,
} from "../../lib/urlstate";
import { type AnalyticsSelection, writableScope } from "./analytics.context";
import { ForecastAnswer } from "./analytics.forecast.answer";
import { ForecastCallEditor } from "./analytics.forecast.calleditor";
import { ForecastDeals, openForecastDeals } from "./analytics.forecast.deals";
import { forecastFit, periodDial } from "./analytics.forecast.fit";
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
  const [params] = useUrlParams();
  // The window and the pipeline come from the address, so a door from the
  // Overview opens on the question its figure answered.
  const named = params.get("pipeline");
  const pipelines = usePipelines(!!named && named !== "all");
  const fit = forecastFit(
    params.get("period"),
    named,
    pipelines.isError ? null : pipelines.data,
  );
  const period = fit.kind === "fits" ? fit.period : "quarter";
  // Whether the call editor is open. Held here rather than in the editor,
  // because the verb that opens it stands in the toolbar beside the period it
  // files against, outside the read the editor needs.
  const [editing, setEditing] = useState(false);
  // Only a reader who may call, calling for ONE population: a blended view of
  // several teams has no single call to record.
  const canCall = canSubmit && writableScope(selection.scope) != null;

  // The same read the morning's pipeline counter makes, through the same key:
  // two surfaces asking what the pipeline is worth must not get two answers.
  const readings = useForecastReadings(
    fit.kind === "fits" ? selection.scope : undefined,
    period,
  );
  if (fit.kind !== "fits")
    return (
      <div className="analytics-stack">
        <FilterBar label={t("analytics.filters")}>{scopeControl}</FilterBar>
        {fit.kind === "pending" ? (
          <PendingBody label={t("analytics.sectionForecast")} />
        ) : (
          <ForecastMismatch kind={fit.kind} />
        )}
      </div>
    );

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
          onChange={choosePeriod}
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
                <ForecastReview scope={selection.scope} />
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

// A window is a dial every Analytics tab reads. Custom dates belong to
// another window, so they go with the change.
function choosePeriod(period: ForecastPeriod): void {
  const next = new Map(currentParams());
  next.set("period", periodDial(period));
  next.delete("from");
  next.delete("through");
  replaceParams(next);
}

// What the forecast cannot narrow to, said before any figure, with the one
// question it can answer instead. Nothing wider is shown until it is chosen.
function ForecastMismatch({ kind }: Readonly<{ kind: "period" | "pipeline" }>) {
  const t = useT();
  return kind === "pipeline" ? (
    <Callout
      tone="warning"
      kind="standing"
      title={t("forecast.mismatch.pipeline")}
      actions={
        <Button onClick={() => replaceDial("pipeline", "all")}>
          {t("forecast.mismatch.allPipelines")}
        </Button>
      }
    >
      <p>{t("forecast.mismatch.pipelineBody")}</p>
    </Callout>
  ) : (
    <Callout
      tone="warning"
      kind="standing"
      title={t("forecast.mismatch.period")}
      actions={
        <Button onClick={() => choosePeriod("quarter")}>
          {t("forecast.mismatch.quarter")}
        </Button>
      }
    >
      <p>{t("forecast.mismatch.periodBody")}</p>
    </Callout>
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
