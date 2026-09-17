// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal head's subtitle and facts strip, the same shapes companyheaderfacts.tsx
// and contactpage.tsx's own ContactSubtitle/ContactFacts draw for their records.
//
// It replaces DealIdentityLine, which read the identical facts as one running,
// dot-joined sentence under the name. A reader compared a deal's value against
// the one beside it by re-parsing a sentence each time; a strip of named cells
// is what every other record page already answered that with.

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { useInstallationSettings } from "../../app/uploadlimit";
import { Popover } from "../../design-system/popover";
import { FieldGuard } from "../../design-system/rbac";
import { Fact, RecordFacts } from "../../design-system/recordfacts";
import { ProvenanceTag } from "../../design-system/trust";
import {
  calendarDaysBetween,
  formatDayMonth,
  formatMoney,
  formatNumber,
} from "../../format/format";
import { type Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { provenanceOf, useViewerId } from "../common";
import {
  type WonWithoutContract,
  WonWithoutContractFact,
} from "../dealwinreason";
import {
  EntityRef,
  rosterOwnerName,
  useRoster,
  useRosterPartial,
} from "../entityref";
import { FxLine } from "./dealcockpit";

type Deal = components["schemas"]["Deal"];

// What the head reads off a deal. Every field is optional because every one
// of them is a fact a deal can lack or a reader can be refused — the same
// contract DealIdentityLine kept.
export type DealIdentity = Partial<
  Pick<
    Deal,
    | "amount_minor"
    | "currency"
    | "stage_id"
    | "expected_close_date"
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
    | "captured_by"
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
      <div className="record-sub record-sub-inline">
        {t("create.relatedCompany")} <FieldGuard mode="masked" />
      </div>
    );
  }
  if (!deal.company_id) {
    return null;
  }
  return (
    <div className="record-sub record-sub-inline">
      <EntityRef kind="company" id={deal.company_id} />
    </div>
  );
}

/**
 * DealIdentityFacts is the head's facts strip: what the deal is worth, where
 * it sits on the board, when it is due, whose deal it is, which account it
 * is on and how it reached Margince — the same six a rep asked for on every
 * other record's head. A partner and how a contract-less deal was won follow
 * when the deal carries either; they used to stand on the identity line for
 * the same reason DealDetails argues for the partner alone — a fact a
 * commission is computed from has to be visible on the record it came from.
 */
export function DealIdentityFacts({
  deal,
  stages,
  locale,
}: Readonly<{
  deal: DealIdentity;
  // The stages the page already sorted for the board, not a second read.
  stages: readonly { id: string; name: string }[];
  locale: Locale;
}>): ReactNode {
  const t = useT();
  const zone = useRecordZone();
  const viewerId = useViewerId();
  // The installation's own reporting currency, read here rather than handed
  // down: only this cell converts, and an unnamed base is not a euro base.
  const baseCurrency = useInstallationSettings().data?.base_currency ?? null;
  // Only asked for when there is an owner to name: an unowned deal needs no
  // roster read to say so.
  const roster = useRoster("user", Boolean(deal.owner_id));
  const partial = useRosterPartial("user", Boolean(deal.owner_id));
  const masked = deal.masked_fields ?? [];
  // An em dash rather than the stage id: a deal whose stage was archived out
  // from under it has no row to name, and printing a UUID where a stage name
  // goes reads as a fault.
  const stage = stages.find((candidate) => candidate.id === deal.stage_id);
  return (
    <RecordFacts>
      <Fact label={t("deals.amount")}>
        {/* A masked amount NAMES the field, like every other refusal on this
            strip: a lone lock among named cells says only that something
            here is hidden, and "no value recorded" and "you may not see the
            value" are different statements about a deal. */}
        {masked.includes("amount_minor") ? (
          <FieldGuard mode="masked" />
        ) : (
          <span className="deal-fact-close">
            {dealAmount(deal, locale)}
            {/* What the figure is worth in the money the installation
                reports in, where the deal is written in another. A rate is
                frozen at close on a won or lost deal, so the line names the
                day it was taken: a converted figure with no date is a number
                nobody can check. */}
            {deal.fx_rate_to_base && (
              <FxLine
                amountMinor={deal.amount_minor ?? null}
                baseCurrency={baseCurrency}
                fxRateToBase={deal.fx_rate_to_base}
                fxRateDate={deal.fx_rate_date ?? null}
                locale={locale}
              />
            )}
          </span>
        )}
      </Fact>
      <Fact label={t("deals.stage")}>{stage?.name ?? "—"}</Fact>
      {/* Always, date or none: when a deal lands is one of the facts a reader
          checks first, and a cell that disappears while nobody has said reads
          as a deal with no question outstanding. */}
      <Fact label={t("deal.strip.close")}>
        <CloseReading deal={deal} locale={locale} zone={zone} />
      </Fact>
      <Fact label={t("list.owner")}>
        {rosterOwnerName(
          deal.owner_id,
          roster,
          partial,
          t,
          t("co.pulse.unowned"),
        )}
      </Fact>
      {masked.includes("company_id") ? (
        <Fact label={t("create.relatedCompany")}>
          <FieldGuard mode="masked" />
        </Fact>
      ) : (
        deal.company_id && (
          <Fact label={t("create.relatedCompany")}>
            <EntityRef kind="company" id={deal.company_id} />
          </Fact>
        )
      )}
      <Fact label={t("history.field.source")}>
        <Popover
          label={
            <ProvenanceTag
              provenance={provenanceOf(deal.captured_by, viewerId)}
            />
          }
        >
          <p className="t-body">{deal.source || t("trust.sourceUnknown")}</p>
        </Popover>
      </Fact>
      {masked.includes("partner_company_id") ? (
        // No attribution word here: what the partner did is withheld WITH
        // the partner, so naming one would decide what a partner nobody
        // could see is owed.
        <Fact label={t("deal.partnerCompany")}>
          <FieldGuard mode="masked" />
        </Fact>
      ) : (
        deal.partner_company_id && (
          <Fact label={t("deal.partnerCompany")}>
            {/* Sourced and influenced are paid differently, so the cell says
                which one rather than a neutral company name that hides the
                distinction the commission turns on. */}
            {t(
              deal.partner_attribution === "influenced"
                ? "deal.partnerInfluenced"
                : "deal.partnerSourced",
            )}{" "}
            <EntityRef kind="company" id={deal.partner_company_id} />
          </Fact>
        )
      )}
      {deal.status === "won" && deal.won_without_contract_reason && (
        <Fact label={t("deals.winReason")}>
          <WonWithoutContractFact deal={deal} />
        </Fact>
      )}
    </RecordFacts>
  );
}

// The value, or an em dash when the deal carries none. The masked case is the
// caller's, because a refusal on this strip is written as the field's name
// beside the guard rather than the guard alone.
function dealAmount(deal: DealIdentity, locale: Locale): ReactNode {
  if (deal.amount_minor == null || !deal.currency) {
    return "—";
  }
  return formatMoney(deal.amount_minor, deal.currency, locale);
}

/**
 * When the deal is expected to close, and how much that date is worth
 * believing.
 *
 * `close_date_provisional` is why the qualifier line exists: it means the
 * nightly run replaced a date that had aged into the past and nobody has
 * confirmed the replacement, and a machine's guess drawn exactly like a date
 * agreed with the buyer is the quiet kind of wrong.
 */
function CloseReading({
  deal,
  locale,
  zone,
}: Readonly<{ deal: DealIdentity; locale: Locale; zone: string }>) {
  const t = useT();
  if (!deal.expected_close_date) {
    return (
      <span className="deal-fact-unset">{t("deal.strip.close.none")}</span>
    );
  }
  const days = calendarDaysBetween(
    new Date(),
    new Date(deal.expected_close_date),
  );
  const parts: string[] = [
    days < 0
      ? t("deal.strip.close.overdue", {
          days: formatNumber(Math.abs(days), locale),
        })
      : t("deal.strip.close.inDays", { days: formatNumber(days, locale) }),
  ];
  if (deal.close_date_provisional) {
    parts.push(t("deal.strip.close.provisional"));
  }
  if (deal.forecast_category) {
    parts.push(forecastLabel(deal.forecast_category, t));
  }
  if (deal.wait_until) {
    parts.push(
      t("deal.strip.close.waiting", {
        date: formatDayMonth(deal.wait_until, locale, zone),
      }),
    );
  }
  return (
    <span className="deal-fact-close">
      <span
        className={
          deal.close_date_provisional || days < 0
            ? "deal-fact-close-warn"
            : undefined
        }
      >
        {formatDayMonth(deal.expected_close_date, locale, zone)}
      </span>
      <span className="t-caption">{parts.join(" · ")}</span>
    </span>
  );
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

function forecastLabel(value: string, t: (key: MessageKey) => string): string {
  return Object.hasOwn(FORECAST_LABELS, value)
    ? t(FORECAST_LABELS[value])
    : value.replaceAll("_", " ");
}
