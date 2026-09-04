import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Card, StatCard } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { EvidenceReceipt } from "../design-system/evidencereceipt";
import { MoneyInput } from "../design-system/moneyinput";
import { StatStrip } from "../design-system/statstrip";
import { formatMoneyOrAbsent, formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import {
  type AnalyticsSelection,
  scopeKey,
  scopeQuery,
  writableScope,
} from "./analytics.context";
import { ForecastReview } from "./analytics.forecast.review";
import { QueryGate, throwProblem } from "./common";

type Readings = components["schemas"]["ForecastReadings"];

// The forecast section: what the period is expected to bring in, what the
// figure does not cover, and what a person believes instead.
//
// The three readings are not equal tiles by accident. A CALL is somebody's
// judgement, EVIDENCE is the part with confirmed dates behind it, and ALREADY
// WON is money that arrived — three different kinds of claim, and a reader who
// takes them for one number has been told something untrue.
export function ForecastView({
  selection,
  canSubmit,
}: Readonly<{ selection: AnalyticsSelection; canSubmit: boolean }>) {
  const t = useT();
  const { locale } = useLocale();

  const readings = useQuery({
    // The population is part of the key: without it a scope change would show
    // the previous population's numbers under the new one's name.
    queryKey: ["forecast", scopeKey(selection.scope)],
    queryFn: async () => {
      const { data, error } = await api.GET("/forecast", {
        params: { query: scopeQuery(selection.scope) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  return (
    <QueryGate query={readings}>
      {(data) => (
        <>
          <ForecastAnswer readings={data} locale={locale} />
          {canSubmit && writableScope(selection.scope) ? (
            <ForecastCallEditor readings={data} selection={selection} />
          ) : null}
          {/* What to check comes BEFORE the receipt: a manager with ten
              minutes reads what needs doing first, and the receipt is what
              they consult when a number looks wrong. */}
          <ForecastReview />
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
        </>
      )}
    </QueryGate>
  );
}

// The answer, in one sentence and then in three readings.
function ForecastAnswer({
  readings,
  locale,
}: Readonly<{ readings: Readings; locale: Locale }>) {
  const t = useT();
  const currency = readings.base_currency;
  const money = (minor: number | null | undefined) =>
    formatMoneyOrAbsent(minor ?? null, currency, locale);

  return (
    <>
      <Callout tone="info" title={t("forecast.question")}>
        {/* The call and the supported figure, and the gap between them. A
            reader shown only one of the two has no way to tell whether the
            call is ahead of the evidence or behind it. */}
        {readings.current_call
          ? t("forecast.answerWithCall", {
              call: money(readings.current_call.amount_minor),
              evidence: money(readings.evidence_minor),
            })
          : t("forecast.answerNoCall", {
              evidence: money(readings.evidence_minor),
            })}
      </Callout>

      {/* An unpriced deal is real pipeline contributing zero money, so the gap
          between eligible and priced is stated beside the total rather than
          left in the receipt alone. */}
      {readings.priced_count < readings.eligible_count && (
        <Callout tone="warn" title={t("forecast.partialTitle")}>
          {t("forecast.partial", {
            priced: formatNumber(readings.priced_count, locale),
            eligible: formatNumber(readings.eligible_count, locale),
          })}
        </Callout>
      )}

      <StatStrip>
        <StatCard
          label={t("forecast.currentCall")}
          value={money(readings.current_call?.amount_minor)}
          numeric
        />
        <StatCard
          label={t("forecast.evidence")}
          value={money(readings.evidence_minor)}
          numeric
        />
        <StatCard
          label={t("forecast.alreadyWon")}
          value={money(readings.won_minor)}
          numeric
        />
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
}: Readonly<{ readings: Readings; selection: AnalyticsSelection }>) {
  const t = useT();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
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
          // The forecast is published for the population the reader is
          // LOOKING at. Hard-coded to the workspace, this recorded a
          // company-wide belief while a manager read their own team's numbers
          // — the assertion and the figure it was formed from disagreeing.
          period: "quarter",
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
      setOpen(false);
      setNote("");
    },
  });

  if (!open) {
    return (
      <div className="card-actions">
        <Button small onClick={() => setOpen(true)}>
          {t("forecast.updateCall")}
        </Button>
      </div>
    );
  }

  return (
    <Card title={t("forecast.updateCall")}>
      <p className="sub">{t("forecast.callExplains")}</p>
      <label className="field">
        <span>{t("forecast.expectedTotal")}</span>
        <MoneyInput
          valueMinor={amountMinor}
          currency={readings.base_currency}
          onChangeMinor={(next) => setAmountMinor(next ?? 0)}
        />
      </label>
      <label className="field">
        <span>{t("forecast.supportingNote")}</span>
        <input
          type="text"
          value={note}
          onChange={(event) => setNote(event.target.value)}
        />
      </label>
      <div className="card-actions">
        <Button small onClick={() => setOpen(false)}>
          {t("forecast.cancel")}
        </Button>
        <Button
          small
          variant="primary"
          disabled={save.isPending}
          onClick={() => save.mutate({ amountMinor, note })}
        >
          {t("forecast.saveCall")}
        </Button>
      </div>
    </Card>
  );
}
