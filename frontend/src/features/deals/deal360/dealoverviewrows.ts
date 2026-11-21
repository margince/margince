// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import type { OccurredPrecision } from "../../../format/activitywhen";
import { formatDayMonth, formatMoneyOrAbsent } from "../../../format/format";
import { offerStatusLabel } from "../../../format/offerstatus";
import type { Locale, Translator } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { committeeContacts, isGapRisk } from "./dealcommittee";

type Activity = components["schemas"]["Activity"];
type Offer = components["schemas"]["Offer"];
type DealCoverage = components["schemas"]["DealCoverage"];
type DealCoverageRisk = components["schemas"]["DealCoverageRisk"];

export type OfferRow = Readonly<{
  id: string;
  title: string;
  sub: string;
  statusLabel: string;
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
      net: formatMoneyOrAbsent(
        offer.money_withheld ? null : offer.net_minor,
        offer.currency,
        locale,
      ),
    }),
    statusLabel: offerStatusLabel(offer.status, t),
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
  // The role a stakeholder added for this gap starts with, when the rule
  // names one.
  role?: string;
}>;

/** When a seat last wrote, drawn by the caller in its locale and zone. */
export type SeatWroteOn = (at: string, precision: OccurredPrecision) => string;

/**
 * The seats the coverage read names, then one row for each seat its rules
 * call missing. A gap names the seat its rule measures: the champion, or a
 * second contact. No rule says which other buying role is absent, so none is
 * guessed.
 */
export function committeeRows(
  coverage: DealCoverage | undefined,
  t: Translator,
  // A stakeholder role in the reader's words.
  roleLabel: (role: string) => string,
  wroteOn?: SeatWroteOn,
): Readonly<{ seats: CommitteeSeatRow[]; gaps: CommitteeGapRow[] }> {
  const contacts = committeeContacts(coverage?.stakeholders ?? []);
  const seats = contacts.map((seat) => {
    const roles = seat.roles.map(roleLabel).filter(Boolean);
    const words = {
      role: roles.length > 0 ? roles.join(", ") : t("deal.committee.noRole"),
      engagement: t(
        seat.engaged
          ? "deal.committee.legendEngaged"
          : "deal.committee.legendQuiet",
      ),
    };
    const wrote =
      seat.last_inbound_at && wroteOn
        ? wroteOn(
            seat.last_inbound_at,
            seat.last_inbound_precision ?? "instant",
          )
        : undefined;
    return {
      key: seat.contact_id,
      name: seat.contact_name ?? undefined,
      sub: wrote
        ? t("deal.committee.seatWrote", { ...words, date: wrote })
        : t("deal.committee.seatSub", words),
      engaged: seat.engaged,
    };
  });
  // The committee dim counts gaps with the same predicate, so the rows and the
  // dim agree.
  const gaps = (coverage?.risks ?? []).filter(isGapRisk).map((risk, index) => {
    const seat = missingSeat(
      risk.kind,
      contacts.length,
      coverage?.max_engaged_seats,
    );
    return {
      key: `${risk.kind}-${index}`,
      kind: risk.kind,
      title: t(seat.title),
      role: seat.role,
    };
  });
  return { seats, gaps };
}

// The seat a gap rule finds missing. A deal with no more seats than the
// published threshold allows engaged is missing a contact; past that it is
// missing an exchange. "A second" holds only at the default threshold of one.
function missingSeat(
  kind: DealCoverageRisk["kind"],
  seatCount: number,
  maxEngaged: number | undefined,
): Readonly<{ title: MessageKey; role?: string }> {
  if (kind === "coverage_gap") {
    return { title: "deal.committee.missing.champion", role: "champion" };
  }
  if (seatCount === 0) {
    return { title: "deal.committee.missing.firstContact" };
  }
  const second = maxEngaged === 1;
  if (maxEngaged === undefined || seatCount > maxEngaged) {
    return {
      title: second
        ? "deal.committee.missing.secondEngaged"
        : "deal.committee.missing.anotherEngaged",
    };
  }
  return {
    title: second
      ? "deal.committee.missing.secondContact"
      : "deal.committee.missing.anotherContact",
  };
}
