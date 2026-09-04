// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The buying committee, drawn.
//
// The rail already LISTS the seats (DealSeats), and this does not repeat that
// list to be pretty. It draws the one thing a list cannot show: the shape of
// the coverage, and the holes in it. A reader counts the threads between our
// column and theirs and sees single-threading as a picture — one line where
// there should be several — before they have read a single name.
//
// A GHOST seat is the point of the map. `coverage_gap` and the two
// single-threaded rules say a seat is MISSING, and a missing thing has no row
// in a list of what exists. Here it is a dashed empty circle, which is the
// only rendering of absence that a reader can count.

import type { components } from "../../api/schema";
import { Badge, Card } from "../../design-system/atoms";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { dealRoleLabel } from "../record360";
import "./dealcommittee.css";
import { SeatPerson } from "./seatperson";

type DealCoverage = components["schemas"]["DealCoverage"];
type DealCoverageSeat = components["schemas"]["DealCoverageSeat"];
type DealCoverageRisk = components["schemas"]["DealCoverageRisk"];

// The map's geometry, in its own coordinates. Two columns, a fixed row pitch:
// the picture is deterministic, so the same deal draws the same map on every
// read.
const WIDTH = 320;
const OURS_X = 64;
const THEIRS_X = 256;
const TOP = 34;
const PITCH = 30;
const SEAT_RADIUS = 9;
const MIN_HEIGHT = 120;
// How far our side's node may grow with the colleagues carrying the deal. It
// is capped because past a few people the node stops reading as a node.
const MAX_OURS_LIFT = 6;

// The risk kinds that mean a seat is missing rather than cold. Each draws one
// ghost on the buyer's column, because that is where the gap is.
const GAP_KINDS: readonly DealCoverageRisk["kind"][] = [
  "coverage_gap",
  "single_threaded_theirs",
];

/** ghostCount reports how many seats the coverage rules say are missing. */
export function ghostCount(risks: readonly DealCoverageRisk[]): number {
  return risks.filter((risk) => GAP_KINDS.includes(risk.kind)).length;
}

/** ghostRows names the rows the ghosts occupy, below the seats that exist. */
function ghostRows(seatCount: number, ghosts: number): number[] {
  return Array.from({ length: ghosts }, (_, offset) => seatCount + offset);
}

/**
 * DealCommitteeMap draws our side against the buying committee.
 *
 * Decorative: `aria-hidden`, nothing focusable. Every seat in it is a row in
 * the accessible list underneath, and the risks are chips a reader can read.
 * The map is the explanation of the list, never a replacement for it.
 */
export function DealCommitteeMap({
  coverage,
  withheld,
  pending,
  overlay,
}: Readonly<{
  coverage?: DealCoverage;
  withheld: boolean;
  pending: boolean;
  // Overlay mode serves a mirrored deal whose coverage this installation
  // cannot assemble, so no seats will ever arrive and the map says so rather
  // than drawing an empty committee.
  overlay: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const seats = coverage?.stakeholders ?? [];
  const ours = coverage?.our_side ?? [];
  const risks = coverage?.risks ?? [];
  const ghosts = ghostCount(risks);
  // Withheld, empty, still loading and FAILED are four different answers, and
  // the primitive is what keeps them apart. A read that errored leaves
  // `coverage` undefined, and a hand-rolled chain over `seats.length` would
  // land on "nobody is on this deal" — a finding from a check that never ran,
  // contradicting the rail two feet away. `sectionState` answers
  // `unavailable` there instead.
  const state = overlay
    ? ("unsupported" as const)
    : sectionState(
        // undefined WHILE PENDING, because that is the only input from which
        // sectionState can answer "loading": it reads the flag solely on the
        // `!view` arm. Handing it a literal either way made `pending` dead
        // here, and a read still in flight rendered "unavailable" — the same
        // sentence a FAILED read gets, which is the distinction the comment
        // above says this primitive exists to keep.
        pending
          ? undefined
          : withheld
            ? { sections_omitted: ["stakeholders"] }
            : { sections_omitted: [] },
        "stakeholders",
        Boolean(coverage),
        seats.length,
        pending,
      );

  return (
    <Card className="dc-card" title={t("deal.committee.title")}>
      <SurfaceState
        state={state}
        emptyLabel={t("deal.committee.empty")}
        detail={
          overlay ? { unsupportedReason: t("overlay.unavailable") } : undefined
        }
      >
        <CommitteeSvg seats={seats} ourCount={ours.length} ghosts={ghosts} />
        <ul className="dc-legend">
          <li>
            <span className="dc-swatch dc-swatch-engaged" />
            {t("deal.committee.legendEngaged")}
          </li>
          <li>
            <span className="dc-swatch dc-swatch-quiet" />
            {t("deal.committee.legendQuiet")}
          </li>
          {ghosts > 0 && (
            <li>
              <span className="dc-swatch dc-swatch-gap" />
              {t("deal.committee.legendGap")}
            </li>
          )}
        </ul>
        {/* The accessible rendering of the same seats, in the same order. */}
        <ul className="dc-seats">
          {seats.map((seat) => (
            <li key={seat.person_id} className="dc-seat-row">
              <span className="dc-seat-name">
                <SeatPerson seat={seat} />
              </span>
              <span className="dc-role">{dealRoleLabel(seat.role, t)}</span>
              <Badge tone={seat.engaged ? "success" : undefined}>
                {seat.engaged
                  ? t("deal.committee.engaged")
                  : t("deal.committee.quiet")}
              </Badge>
            </li>
          ))}
        </ul>
        {/* The findings themselves are NOT repeated here. DealSignals
              already renders the same `risks` as localized chips above this
              card, and a second rendering would give one finding two
              spellings — and this one would be the untranslated summary. What
              the map adds is where the gap IS, which the ghosts carry. */}
        <p className="t-caption">
          {t("deal.committee.threads", {
            engaged: formatNumber(
              seats.filter((s) => s.engaged).length,
              locale,
            ),
            total: formatNumber(seats.length, locale),
          })}
        </p>
      </SurfaceState>
    </Card>
  );
}

/**
 * CommitteeSvg draws our column, their column, and a thread for every seat
 * with a two-way exchange.
 *
 * A thread is drawn from the middle of our column rather than from one named
 * colleague: the coverage payload says how many of us carry the deal, not
 * which of us carries which seat, and drawing a line per colleague would
 * invent an attribution the server never made.
 */
function CommitteeSvg({
  seats,
  ourCount,
  ghosts,
}: Readonly<{
  seats: readonly DealCoverageSeat[];
  ourCount: number;
  ghosts: number;
}>) {
  const rows = seats.length + ghosts;
  const height = Math.max(MIN_HEIGHT, TOP + rows * PITCH);
  const ourY = height / 2;
  return (
    <svg
      className="dc-map"
      viewBox={`0 0 ${WIDTH} ${height}`}
      aria-hidden="true"
      focusable="false"
    >
      {seats.map((seat, index) =>
        // Only an engaged seat carries a thread. A line to a seat nobody has
        // exchanged with would draw a conversation that never happened, which
        // is the exact claim this map exists to disprove.
        seat.engaged ? (
          <line
            key={`thread-${seat.person_id}`}
            className="dc-thread"
            x1={OURS_X}
            y1={ourY}
            x2={THEIRS_X}
            y2={TOP + index * PITCH}
          />
        ) : null,
      )}
      {/* Our side is ONE node, sized by how many of us carry the deal. A
          number drawn here would be a magnitude in the picture rather than in
          the reader's own notation, and the count is already a sentence in the
          list underneath. */}
      <circle
        className="dc-seat dc-seat-ours"
        cx={OURS_X}
        cy={ourY}
        r={SEAT_RADIUS + Math.min(ourCount, MAX_OURS_LIFT)}
      />
      {seats.map((seat, index) => (
        <circle
          key={seat.person_id}
          className={`dc-seat ${seat.engaged ? "dc-seat-engaged" : "dc-seat-quiet"}`}
          cx={THEIRS_X}
          cy={TOP + index * PITCH}
          r={SEAT_RADIUS}
        />
      ))}
      {/* The gaps, drawn after the real seats so they read as the tail of the
          committee rather than as members of it. A ghost has no identity to
          key on — it is a seat nobody filled — so its row is its key. */}
      {ghostRows(seats.length, ghosts).map((row) => (
        <circle
          key={`ghost-row-${row}`}
          className="dc-ghost"
          cx={THEIRS_X}
          cy={TOP + row * PITCH}
          r={SEAT_RADIUS}
        />
      ))}
    </svg>
  );
}
