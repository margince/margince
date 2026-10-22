// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The buying committee, drawn. Our owner is on the left, with a curve to every
// seat on the buyer's side. A dashed seat marks every gap the coverage rules
// find. A missing seat has no row in a list of what exists; here it is a ring
// a reader can count.

import type { components } from "../../../api/schema";
import {
  SurfaceState,
  sectionState,
} from "../../../design-system/surfacestate";
import { formatNumber } from "../../../format/format";
import { type Locale, type Translator, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { navigate } from "../../../lib/router";
import { committeeRows } from "./dealoverviewrows";
import { committeeOf } from "./dealstanding";
import "./dealcommittee.css";

type DealCoverage = components["schemas"]["DealCoverage"];
type DealCoverageRisk = components["schemas"]["DealCoverageRisk"];

// One row's height, in the picture's own units. The CSS sets the seat rows to
// the same pitch (`--dc-pitch`), so every curve lands on its row's dot.
const PITCH = 52;
const LINK_WIDTH = 72;
// From the owner's dot (centred in its 72px column, 16px wide) to the
// picture's left edge, so every curve leaves from the dot itself.
const FROM_DOT = 28;

/** isGapRisk says whether a coverage finding means a seat is missing. */
export function isGapRisk(risk: DealCoverageRisk): boolean {
  return risk.kind === "coverage_gap" || risk.kind === "single_threaded_theirs";
}

/** ghostCount reports how many seats the coverage rules say are missing. */
export function ghostCount(risks: readonly DealCoverageRisk[]): number {
  return risks.filter(isGapRisk).length;
}

// What each gap row says under its name: the rule's own finding, never a role
// nobody measured.
const GAP_SUB: Partial<Record<DealCoverageRisk["kind"], MessageKey>> = {
  coverage_gap: "deal.committee.gap.coverageGap",
  single_threaded_theirs: "deal.committee.gap.singleThreaded",
};

type Link = Readonly<{ key: string; row: number; gap: boolean }>;

/**
 * CommitteeReading draws the deal's owner against the buying committee, with
 * the legend and the count under it.
 */
export function CommitteeReading({
  coverage,
  withheld,
  pending,
  owner,
  roleLabel,
}: Readonly<{
  coverage?: DealCoverage;
  withheld: boolean;
  pending: boolean;
  // The deal owner's name, for our side of the picture.
  owner?: string;
  roleLabel: (role: string) => string;
}>) {
  const t = useT();
  const { seats, gaps } = committeeRows(coverage, t, roleLabel);
  const state = sectionState(
    // Undefined while pending: it is the only input sectionState reads as
    // "loading" rather than "unavailable".
    pending
      ? undefined
      : withheld
        ? { sections_omitted: ["stakeholders"] }
        : { sections_omitted: [] },
    "stakeholders",
    Boolean(coverage),
    seats.length + gaps.length,
    pending,
  );
  // Only an engaged seat and a gap carry a curve. A line to a seat nobody has
  // exchanged with would draw a conversation that never happened.
  const links: Link[] = [
    ...seats.flatMap((seat, row) =>
      seat.engaged ? [{ key: seat.key, row, gap: false }] : [],
    ),
    ...gaps.map((gap, offset) => ({
      key: gap.key,
      row: seats.length + offset,
      gap: true,
    })),
  ];
  return (
    <SurfaceState
      loadingLabel={t("deal.committee.title")}
      state={state}
      emptyLabel={t("deal.standing.committee.none")}
    >
      <div className="dc-graph">
        <div className="dc-us">
          <span className="dc-us-dot" aria-hidden="true" />
          <span className="dc-us-words">
            {owner && <span className="dc-us-name">{owner}</span>}
            <span className="dc-us-role">{t("deal.committee.owner")}</span>
          </span>
        </div>
        <CommitteeLinks rows={seats.length + gaps.length} links={links} />
        <ul className="dc-seats">
          {seats.map((seat) => (
            <li key={seat.key} className="dc-seat">
              <button
                type="button"
                className="dc-seat-open"
                onClick={() => navigate({ screen: "contacts", id: seat.key })}
              >
                <span
                  className={`dc-dot ${seat.engaged ? "dc-dot-engaged" : "dc-dot-quiet"}`}
                  aria-hidden="true"
                />
                <span className="dc-seat-text">
                  <span className="dc-seat-name">
                    {seat.name ?? t("coverage.seatWithheld")}
                  </span>
                  <span className="dc-seat-sub">{seat.sub}</span>
                </span>
              </button>
            </li>
          ))}
          {gaps.map((gap) => {
            const sub = GAP_SUB[gap.kind];
            return (
              <li
                key={gap.key}
                className="dc-seat dc-seat-gap"
                title={sub && t(sub)}
              >
                <span className="dc-dot dc-dot-gap" aria-hidden="true" />
                <span className="dc-seat-text">
                  <span className="dc-seat-name">{gap.title}</span>
                  {sub && <span className="dc-seat-sub">{t(sub)}</span>}
                </span>
              </li>
            );
          })}
        </ul>
      </div>
      <ul className="dc-legend">
        <li>
          <span className="dc-swatch dc-swatch-engaged" />
          {t("deal.committee.legendEngaged")}
        </li>
        <li>
          <span className="dc-swatch dc-swatch-quiet" />
          {t("deal.committee.legendQuiet")}
        </li>
        <li>
          <span className="dc-swatch dc-swatch-gap" />
          {t("deal.committee.legendGap")}
        </li>
      </ul>
    </SurfaceState>
  );
}

/**
 * The line under the committee: how many are named and how many of them we
 * are in touch with. It is in warning ink while a seat is missing or one seat
 * carries the deal. None until the read has named somebody.
 */
export function committeeFoot(
  coverage: DealCoverage | undefined,
  withheld: boolean,
  t: Translator,
  locale: Locale,
): Readonly<{ text: string; warn: boolean }> | undefined {
  const committee = committeeOf(coverage, withheld);
  if (committee.state !== "read" || committee.named === 0) {
    return undefined;
  }
  return {
    text: t("deal.committee.foot", {
      named: formatNumber(committee.named, locale),
      engaged: formatNumber(committee.engaged, locale),
    }),
    warn: committee.gaps > 0 || committee.named <= 1,
  };
}

// The curves from our owner to the seats. Decorative: every seat is a row
// beside it.
function CommitteeLinks({
  rows,
  links,
}: Readonly<{ rows: number; links: readonly Link[] }>) {
  // Two rows at least, so our owner's name has room under the dot; the
  // seats are centred in that height, and so are their curves.
  const height = Math.max(rows, 2) * PITCH;
  const fromY = height / 2;
  const top = (height - rows * PITCH) / 2;
  const mid = LINK_WIDTH / 2;
  return (
    <svg
      className="dc-links"
      viewBox={`0 0 ${LINK_WIDTH} ${height}`}
      width={LINK_WIDTH}
      height={height}
      aria-hidden="true"
      focusable="false"
    >
      {links.map((link) => {
        const toY = top + link.row * PITCH + PITCH / 2;
        return (
          <path
            key={link.key}
            className={link.gap ? "dc-link dc-link-gap" : "dc-link"}
            d={`M${-FROM_DOT} ${fromY} C${mid} ${fromY} ${mid} ${toY} ${LINK_WIDTH} ${toY}`}
          />
        );
      })}
    </svg>
  );
}
