// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import { formatDayMonth, formatMoney } from "../../../format/format";
import { offerStatusLabel } from "../../../format/offerstatus";
import type { Locale, Translator } from "../../../i18n";
import { isGapRisk } from "./dealcommittee";

type Activity = components["schemas"]["Activity"];
type Offer = components["schemas"]["Offer"];
type DealCoverage = components["schemas"]["DealCoverage"];
type DealCoverageRisk = components["schemas"]["DealCoverageRisk"];

export type OfferRow = Readonly<{
  id: string;
  title: string;
  sub: string;
  status: string;
}>;

/**
 * One offer as a card row. The wire has no sent date, so the row dates the
 * offer by when it was written and says so.
 */
export function offerRow(
  offer: Offer,
  t: Translator,
  locale: Locale,
  zone: string,
): OfferRow {
  return {
    id: offer.id,
    title: t("deal.offers.rowTitle", {
      number: offer.offer_number,
      revision: String(offer.revision),
    }),
    sub: t("deal.offers.rowSub", {
      date: formatDayMonth(offer.created_at, locale, zone),
      net: formatMoney(offer.net_minor, offer.currency, locale),
    }),
    status: offerStatusLabel(offer.status, t),
  };
}

/**
 * The open tasks on the deal beyond the one the Next bar names. The thread is
 * a page, so a count read off a page with more behind it is a floor.
 */
export function otherOpenTaskRows(
  activities: readonly Activity[],
  shownIds: readonly string[],
): Activity[] {
  return activities.filter(
    (row) =>
      row.kind === "task" &&
      !row.is_done &&
      !row.archived_at &&
      !shownIds.includes(row.id),
  );
}

export type CommitteeSeatRow = Readonly<{
  key: string;
  name?: string;
  sub: string;
  engaged: boolean;
}>;

export type CommitteeGapRow = Readonly<{
  key: string;
  kind: DealCoverageRisk["kind"];
  title: string;
}>;

/**
 * The seats the coverage read names, then one row for each seat its rules
 * call missing. A missing seat is named by the rule that found it: no rule
 * says which buying role is absent, so none is guessed.
 */
export function committeeRows(
  coverage: DealCoverage | undefined,
  t: Translator,
  // A stakeholder role in the reader's words.
  roleLabel: (role: string) => string,
): Readonly<{ seats: CommitteeSeatRow[]; gaps: CommitteeGapRow[] }> {
  const seats = (coverage?.stakeholders ?? []).map((seat) => ({
    key: seat.contact_id,
    name: seat.contact_name ?? undefined,
    // The wire carries no date of the last exchange, so an engaged seat says
    // only that it is engaged.
    sub: seat.engaged
      ? t("deal.committee.seatSub", {
          role: roleLabel(seat.role),
          engagement: t("deal.committee.legendEngaged"),
        })
      : roleLabel(seat.role),
    engaged: seat.engaged,
  }));
  // The committee dim counts gaps with the same predicate, so the rows and the
  // dim agree.
  const gaps = (coverage?.risks ?? []).filter(isGapRisk).map((risk, index) => ({
    key: `${risk.kind}-${index}`,
    kind: risk.kind,
    title: t(`coverage.risk.${risk.kind}`),
  }));
  return { seats, gaps };
}
