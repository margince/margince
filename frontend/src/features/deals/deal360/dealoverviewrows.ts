// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import type { OccurredPrecision } from "../../../format/activitywhen";
import { formatDayMonth, formatMoney } from "../../../format/format";
import { offerStatusLabel } from "../../../format/offerstatus";
import type { Locale, Translator } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
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
  const stakeholders = coverage?.stakeholders ?? [];
  const seats = stakeholders.map((seat) => {
    const words = {
      role: seat.role ? roleLabel(seat.role) : t("deal.committee.noRole"),
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
    const seat = missingSeat(risk.kind, stakeholders.length);
    return {
      key: `${risk.kind}-${index}`,
      kind: risk.kind,
      title: t(seat.title),
      role: seat.role,
    };
  });
  return { seats, gaps };
}

// The floor `single_threaded_theirs` holds engaged contacts to.
const THREADING_FLOOR = 2;

// The seat a gap rule finds missing. Fewer than two engaged is a missing
// contact while the deal seats fewer than two, and a missing exchange past that.
function missingSeat(
  kind: DealCoverageRisk["kind"],
  seatCount: number,
): Readonly<{ title: MessageKey; role?: string }> {
  if (kind === "coverage_gap") {
    return { title: "deal.committee.missing.champion", role: "champion" };
  }
  if (seatCount === 0) {
    return { title: "deal.committee.missing.firstContact" };
  }
  return {
    title:
      seatCount < THREADING_FLOOR
        ? "deal.committee.missing.secondContact"
        : "deal.committee.missing.secondEngaged",
  };
}
