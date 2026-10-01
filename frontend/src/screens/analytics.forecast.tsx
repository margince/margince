import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import {
  Button,
  Disclosure,
  Field,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { EvidenceReceipt } from "../design-system/evidencereceipt";
import { MoneyInput } from "../design-system/moneyinput";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SegmentBar } from "../design-system/readings";
import { StatStrip } from "../design-system/statstrip";
import {
  formatDateAbbrev,
  formatDateTime,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { formatMoneyOrWord } from "../format/moneyword";
import { type Locale, useLocale, useT } from "../i18n";
import { type AnalyticsSelection, writableScope } from "./analytics.context";
import { LandingCard, SufficiencyCard } from "./analytics.forecast.landing";
import { ForecastReview } from "./analytics.forecast.review";
import { QueryGate, throwProblem } from "./common";
import {
  FORECAST_PERIODS,
  type ForecastPeriod,
  useForecastReadings,
} from "./forecast.queries";
import { ReportingForecastGraphs } from "./reporting.forecast";
import "./analytics.css";

type Readings = components["schemas"]["ForecastReadings"];

export function ForecastView({
  selection,
  canSubmit,
  reportingEnabled = false,
}: Readonly<{
  selection: AnalyticsSelection;
  canSubmit: boolean;
  reportingEnabled?: boolean;
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
      <div className="analytics-toolbar">
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
        {canCall && !editing ? (
          <Button onClick={() => setEditing(true)}>
            {t("forecast.updateCall")}
          </Button>
        ) : null}
      </div>
      <QueryGate query={readings} pendingLabel={t("forecast.updateCall")}>
        {(data) => (
          <>
            {data.period_start && data.period_end && (
              <p className="t-caption">
                {formatDateAbbrev(data.period_start, locale, data.timezone)} –{" "}
                {formatDateAbbrev(data.period_end, locale, data.timezone)}
              </p>
            )}
            <ForecastAnswer readings={data} locale={locale} />
            {reportingEnabled && period === "quarter" && (
              <ReportingForecastGraphs scope={selection.scope} />
            )}
            {canCall && editing ? (
              <ForecastCallEditor
                readings={data}
                selection={selection}
                period={period}
                onClose={() => setEditing(false)}
              />
            ) : null}
            {/* What to check comes BEFORE the receipt: a manager with ten
              minutes reads what needs doing first, and the receipt is what
              they consult when a number looks wrong. Side by side where the
              page is wide, so the receipt stops being a full-width table of
              four numbers. */}
            <div className="analytics-pair">
              <ForecastReview />
              <Disclosure summary={t("forecast.receipt")}>
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
              </Disclosure>
            </div>
          </>
        )}
      </QueryGate>
    </div>
  );
}

// Compare the period forecast with won sales plus confirmed committed deals.
function callDetail(
  call: NonNullable<Readings["current_call"]>,
  readings: Readings,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  // The day the call was authored, cut in the zone the period itself was cut
  // in: a reporting figure and the date beside it must not be bucketed on two
  // different calendars.
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

// The answer, in one sentence and then in three readings.
function ForecastAnswer({
  readings,
  locale,
}: Readonly<{ readings: Readings; locale: Locale }>) {
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
  // What best case adds on top of the evidence it already contains. Never
  // below zero: a superset that read smaller than its subset is a server fault
  // to show as nothing added, not as a part drawn backwards.
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
            <p className="t-caption">{callDetail(call, readings, locale, t)}</p>
          )}
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
          box while the rows beside it lost theirs. */}
      <StatStrip>
        {/* Both are absent for a managed-teams reading, which covers several
            populations at once: a landing summed across books that are called
            separately, and a coverage rate blended over them, would each
            describe none of them. */}
        {readings.landing && (
          <LandingCard
            landing={readings.landing}
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
    </>
  );
}

// Recording what somebody believes will close.
//
// It writes no deal row, and the copy says so: a manager who disagrees with the
// derived figure records their own number instead of editing the pipeline until
// the derivation agrees with them.
function ForecastCallEditor({
  readings,
  selection,
  period,
  onClose,
}: Readonly<{
  readings: Readings;
  selection: AnalyticsSelection;
  period: ForecastPeriod;
  // Leaving the editor, whether by cancelling or by a recorded call: the view
  // that opened it owns whether it is open.
  onClose: () => void;
}>) {
  const t = useT();
  const client = useQueryClient();
  const [amountMinor, setAmountMinor] = useState<number>(
    readings.current_call?.amount_minor ?? 0,
  );
  const [note, setNote] = useState("");

  const save = useMutation({
    // Amount and note travel as VARIABLES rather than being read from the
    // closure: a mutation that closes over its inputs sends whatever the last
    // render saw, which is the wrong number exactly when a save races a
    // refetch.
    mutationFn: async (call: { amountMinor: number; note: string }) => {
      const named = writableScope(selection.scope);
      if (!named) {
        // The editor is not rendered without a nameable population, so this is
        // unreachable rather than a state to design copy for.
        throw new Error("a forecast names one population");
      }
      const { data, error } = await api.POST("/forecast/calls", {
        body: {
          // The forecast is published for the population AND the window the
          // reader is LOOKING at. Hard-coded to the workspace, this recorded a
          // company-wide belief while a manager read their own team's numbers
          // — the assertion and the figure it was formed from disagreeing. A
          // hard-coded quarter is the same defect one axis over: a call made
          // while reading the week would be filed against three months.
          period,
          ...named,
          amount_minor: call.amountMinor,
          currency: readings.base_currency,
          // An empty note is no note. Sent as "", it would claim the author
          // wrote something blank.
          note: call.note === "" ? undefined : call.note,
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: async () => {
      // The readings carry the standing call, so they are stale the moment one
      // is recorded. Invalidating the whole "forecast" prefix rather than this
      // population alone: a workspace call changes what a team reading shows
      // beneath it, and a stale sibling is the bug this replaces.
      await client.invalidateQueries({ queryKey: ["forecast"] });
      onClose();
    },
  });

  return (
    <Panel
      title={t("forecast.updateCall")}
      // Cancel and save both leave this editor, so they stand under what they
      // act on rather than in the band that names it.
      actions={
        <>
          <Button onClick={onClose}>{t("forecast.cancel")}</Button>
          <Button
            variant="primary"
            disabled={save.isPending}
            onClick={() => save.mutate({ amountMinor, note })}
          >
            {t("forecast.saveCall")}
          </Button>
        </>
      }
    >
      <PanelBody>
        {/* Two sentences: what a call is, and what recording one does not do.
            The head band holds one line, and the half it would cut is the
            half that says no deal moves. */}
        <PanelIntro>{t("forecast.callExplains")}</PanelIntro>
        <Field label={t("forecast.expectedTotal")}>
          {(control) => (
            <MoneyInput
              {...control}
              valueMinor={amountMinor}
              currency={readings.base_currency}
              onChangeMinor={(next) => setAmountMinor(next ?? 0)}
            />
          )}
        </Field>
        <Field label={t("forecast.supportingNote")}>
          {(control) => (
            <TextInput
              {...control}
              type="text"
              value={note}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </Field>
      </PanelBody>
    </Panel>
  );
}

export function SharedForecastView({ token }: Readonly<{ token: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const query = useQuery({
    queryKey: ["shared-forecast", token],
    queryFn: async () => {
      const { data, error } = await api.GET("/forecast/shared/{token}", {
        params: { path: { token } },
      });
      if (error) throwProblem(error);
      return data;
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
            <ForecastAnswer readings={view.readings} locale={locale} />
          </>
        )}
      </QueryGate>
    </div>
  );
}
