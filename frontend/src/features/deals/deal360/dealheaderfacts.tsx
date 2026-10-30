// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal head's subtitle, the deal's own rows in its details, and the close
// date in words for the Close reading.

import type { ReactNode } from "react";
import type { components } from "../../../api/schema";
import { EntityRef } from "../../../components/entityref";
import { FieldRow } from "../../../design-system/fieldgrid";
import { Popover } from "../../../design-system/popover";
import { provenanceOf } from "../../../design-system/provenanceof";
import { FieldGuard } from "../../../design-system/rbac";
import { ProvenanceTag } from "../../../design-system/trust";
import { calendarDaysUntil } from "../../../format/daysuntil";
import {
  formatDate,
  formatDateAbbrev,
  formatDayMonth,
  formatMoneyOrAbsent,
  formatNumber,
} from "../../../format/format";
import {
  type Locale,
  type Translator,
  translatePlural,
  useT,
} from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { closeDayOf } from "../../../lib/closeday";
import { writtenByLabel } from "../../../lib/leadorigins";
import {
  lostReasonWords,
  useLossReasons,
} from "../../../lib/lossreasons.queries";
import { useRecordZone } from "../../../lib/recordzone";
import { useViewerId } from "../../../lib/session";
import { useInstallationSettings } from "../../../lib/uploadlimit";
import {
  LostReasonFact,
  type WonWithoutContract,
  WonWithoutContractFact,
} from "../dealwinreason";
import { waitHolds } from "./dealstanding";

type Deal = components["schemas"]["Deal"];

// What the details rows and the close words read off a deal. Every field is
// optional because every one is a fact a deal can lack or a reader can be
// refused.
export type DealIdentity = Partial<
  Pick<
    Deal,
    | "amount_minor"
    | "currency"
    | "stage_id"
    | "expected_close_date"
    | "closed_at"
    | "close_date_provisional"
    | "forecast_category"
    | "wait_until"
    | "fx_rate_to_base"
    | "fx_rate_date"
    | "owner_id"
    | "company_id"
    | "partner_company_id"
    | "partner_attribution"
    | "masked_fields"
    | "source"
    | "imported_from"
    | "captured_by"
    | "author"
    | "status"
    | "lost_reason"
    | "lost_reason_code"
  >
> &
  WonWithoutContract;

/**
 * DealSubtitle is the name line's own subtitle: the company this deal
 * belongs to, beside the name rather than under everything the head carries
 * — the same inline shape ContactSubtitle draws for a contact's employer.
 *
 * A masked company NAMES the field it withholds, the same rule the facts
 * strip below keeps: a bare "—" would read as "this deal has no company",
 * which is a different and wrong statement.
 */
export function DealSubtitle({
  deal,
}: Readonly<{
  deal: Pick<Deal, "company_id" | "masked_fields">;
}>): ReactNode {
  const t = useT();
  const masked = deal.masked_fields ?? [];
  if (masked.includes("company_id")) {
    return (
      <div>
        {t("create.relatedCompany")} <FieldGuard mode="masked" />
      </div>
    );
  }
  if (!deal.company_id) {
    return null;
  }
  return (
    <div>
      <EntityRef kind="company" id={deal.company_id} />
    </div>
  );
}

/**
 * DealRecordRows are the deal's facts the edit form beside them does not
 * carry, as rows of the details grid. They are the stage it stands on and the value
 * in the money the installation reports in. They also say how a closed deal
 * closed and how the record reached Margince and when.
 */
export function DealRecordRows({
  deal,
  stages,
  locale,
}: Readonly<{
  deal: DealIdentity & Partial<Pick<Deal, "created_at">>;
  // The stages the page already sorted for the ladder, not a second read.
  stages: readonly { id: string; name: string }[];
  locale: Locale;
}>): ReactNode {
  const t = useT();
  const zone = useRecordZone();
  const viewerId = useViewerId();
  // The installation's own reporting currency: an unnamed base is not a euro
  // base.
  const baseCurrency = useInstallationSettings().data?.base_currency ?? null;
  // A stage archived out from under the deal has no name, and a UUID where a
  // stage name goes reads as a fault.
  const stage = stages.find((candidate) => candidate.id === deal.stage_id);
  return (
    <>
      <FieldRow label={t("deals.stage")}>{stage?.name ?? "—"}</FieldRow>
      {deal.fx_rate_to_base &&
        !deal.masked_fields?.includes("amount_minor") && (
          <FieldRow label={t("deal.standing.baseValue")}>
            <FxLine
              amountMinor={deal.amount_minor ?? null}
              baseCurrency={baseCurrency}
              fxRateToBase={deal.fx_rate_to_base}
              fxRateDate={deal.fx_rate_date ?? null}
              locale={locale}
            />
          </FieldRow>
        )}
      <ClosingRow deal={deal} />
      <FieldRow label={t("deal.recordedBy")}>
        <Popover
          label={
            <ProvenanceTag
              provenance={provenanceOf(
                deal.captured_by,
                viewerId,
                deal.author,
                deal.source,
                deal.imported_from,
              )}
            />
          }
        >
          <p className="t-body">
            {deal.source
              ? writtenByLabel(deal.source, t)
              : t("trust.sourceUnknown")}
          </p>
        </Popover>
      </FieldRow>
      {deal.created_at && (
        <FieldRow label={t("list.created")}>
          {formatDateAbbrev(deal.created_at, locale, zone)}
        </FieldRow>
      )}
    </>
  );
}

// How a closed deal closed: a win with no contract behind it says how it was
// won, a loss says why.
function ClosingRow({ deal }: Readonly<{ deal: DealIdentity }>): ReactNode {
  const t = useT();
  const reasons = useLossReasons(deal.status === "lost").data;
  if (deal.status === "won" && deal.won_without_contract_reason) {
    return (
      <FieldRow label={t("deals.winReason")}>
        <WonWithoutContractFact deal={deal} />
      </FieldRow>
    );
  }
  const lost = lostReasonWords(deal, reasons);
  if (deal.status === "lost" && lost) {
    return (
      <FieldRow label={t("deals.lostReason")}>
        <LostReasonFact reason={lost} />
      </FieldRow>
    );
  }
  return null;
}

/** The close date in words: the day, how far off it is, and what qualifies
 *  it. Shared by the head's cell and the deal's Close reading. */
export type CloseWords = Readonly<{
  date?: string;
  parts: readonly string[];
  // Days until the date, counted only on an open deal.
  daysLeft?: number;
  warn: boolean;
}>;

/**
 * The words for when the deal closes: the day it closed once it is won or lost,
 * the day it is expected to otherwise.
 *
 * Counted in the record's zone, the one the date is drawn in, so the date and
 * the count cannot name different days. A won or lost deal's date is when it
 * closed, so only an open deal gets a count.
 */
export function closeWords(
  deal: DealIdentity,
  locale: Locale,
  zone: string,
  t: Translator,
): CloseWords {
  const closeDay = closeDayOf(deal);
  if (!closeDay) {
    return { parts: [], warn: false };
  }
  const closed = deal.status === "won" || deal.status === "lost";
  const daysLeft = closed ? undefined : calendarDaysUntil(closeDay, zone);
  const parts: string[] = [];
  if (daysLeft !== undefined) {
    parts.push(
      daysLeft < 0
        ? translatePlural(locale, "deal.strip.close.overdue", -daysLeft, {
            days: formatNumber(-daysLeft, locale),
          })
        : translatePlural(locale, "deal.strip.close.inDays", daysLeft, {
            days: formatNumber(daysLeft, locale),
          }),
    );
  }
  // A date the nightly run moved and nobody confirmed is a machine's guess,
  // and must not read like a date agreed with the buyer.
  if (deal.close_date_provisional) {
    parts.push(t("deal.strip.close.provisional"));
  }
  if (deal.forecast_category) {
    parts.push(forecastLabel(deal.forecast_category, t));
  }
  if (waitHolds(deal.wait_until, zone, new Date())) {
    parts.push(
      t("deal.strip.close.waiting", {
        date: formatDayMonth(deal.wait_until, locale, zone),
      }),
    );
  }
  return {
    date: formatDayMonth(closeDay, locale, zone),
    parts,
    daysLeft,
    warn:
      Boolean(deal.close_date_provisional) ||
      (daysLeft !== undefined && daysLeft < 0),
  };
}

// The forecast words, in the reader's language. An unmapped value renders as
// itself: the category is a wire enum, and a newer server naming a fifth is
// still telling this reader something.
const FORECAST_LABELS: Record<string, MessageKey> = {
  commit: "deal.forecast.commit",
  best_case: "deal.forecast.bestCase",
  pipeline: "deal.forecast.pipeline",
  omitted: "deal.forecast.omitted",
};

export function forecastLabel(
  value: string,
  t: (key: MessageKey) => string,
): string {
  const key = Object.hasOwn(FORECAST_LABELS, value)
    ? FORECAST_LABELS[value]
    : undefined;
  return key === undefined ? value.replaceAll("_", " ") : t(key);
}

// The amount, converted into the installation's base currency at the frozen
// rate, with the day the rate was taken. A converted figure with no date is a
// number nobody can check.
export function FxLine({
  amountMinor,
  baseCurrency,
  fxRateToBase,
  fxRateDate,
  locale,
}: Readonly<{
  amountMinor: number | null;
  // The installation's own base currency, from its settings. Not a constant:
  // an installation whose base is not the euro was reading a euro sign over a
  // figure converted into something else. A converted figure must not make
  // that error. Null while the settings read is in flight or refused, because
  // an unnamed base is not a euro base.
  baseCurrency: string | null;
  fxRateToBase: string;
  fxRateDate: string | null;
  locale: Locale;
}>) {
  const t = useT();
  const recordZone = useRecordZone();
  // A deal carrying a rate but no amount converts to nothing, not to zero.
  const baseMinor =
    amountMinor == null ? null : Math.round(amountMinor * Number(fxRateToBase));
  return (
    <p className="t-caption">
      {t("deal.fxBase", {
        value: formatMoneyOrAbsent(baseMinor, baseCurrency, locale),
        rate: fxRateToBase,
        date: fxRateDate ? formatDate(fxRateDate, locale, recordZone) : "—",
      })}
    </p>
  );
}
