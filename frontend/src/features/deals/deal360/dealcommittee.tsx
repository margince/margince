// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The buying committee as two named sides. Our team lists who works the deal
// and their jobs. Their team lists each seat with its role, engagement and last
// message, then each seat the coverage rules find missing. A mark leads
// every row, and the legend names every mark the card draws.

import { type ReactNode, useId } from "react";
import type { components } from "../../../api/schema";
import { Badge } from "../../../design-system/atoms";
import { Eyebrow } from "../../../design-system/eyebrow";
import {
  SurfaceState,
  sectionState,
} from "../../../design-system/surfacestate";
import { formatNumber } from "../../../format/format";
import { type Locale, type Translator, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { navigate } from "../../../lib/router";
import type { CommitteeColleague } from "./dealcommitteeteam";
import {
  type CommitteeGapRow,
  committeeRows,
  type SeatWroteOn,
} from "./dealoverviewrows";
import { committeeOf } from "./dealstanding";
import "./dealcommittee.css";

type DealCoverage = components["schemas"]["DealCoverage"];
type DealCoverageRisk = components["schemas"]["DealCoverageRisk"];

/** isGapRisk says whether a coverage finding means a seat is missing. */
export function isGapRisk(risk: DealCoverageRisk): boolean {
  return risk.kind === "coverage_gap" || risk.kind === "single_threaded_theirs";
}

/** ghostCount reports how many seats the coverage rules say are missing. */
export function ghostCount(risks: readonly DealCoverageRisk[]): number {
  return risks.filter(isGapRisk).length;
}

type Mark = "ours" | "engaged" | "quiet" | "gap";

// The legend's order, which is the order the marks first appear on the card.
const MARKS: readonly Mark[] = ["ours", "engaged", "quiet", "gap"];

const MARK_WORDS: Record<Mark, MessageKey> = {
  ours: "deal.committee.ourTeam",
  engaged: "deal.committee.legendEngaged",
  quiet: "deal.committee.legendQuiet",
  gap: "deal.committee.legendGap",
};

/** Seats a stakeholder for one gap; `describedBy` names the gap's words. */
export type AddSeat = (gap: CommitteeGapRow, describedBy: string) => ReactNode;

/**
 * CommitteeReading draws both sides of the deal with the legend under them.
 * `addSeat` is absent for a reader who may not add a stakeholder.
 */
export function CommitteeReading({
  coverage,
  withheld,
  pending,
  team,
  teamPending,
  teamUnread,
  roleLabel,
  wroteOn,
  addSeat,
}: Readonly<{
  coverage?: DealCoverage;
  withheld: boolean;
  pending: boolean;
  team: readonly CommitteeColleague[];
  // The read of who works the deal is out, or failed, so the list may be short.
  teamPending?: boolean;
  teamUnread?: boolean;
  roleLabel: (role: string) => string;
  wroteOn?: SeatWroteOn;
  addSeat?: AddSeat;
}>) {
  const t = useT();
  const oursId = useId();
  const theirsId = useId();
  const { seats, gaps } = committeeRows(coverage, t, roleLabel, wroteOn);
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
  const shown: Record<Mark, boolean> = {
    ours: team.length > 0,
    engaged: seats.some((seat) => seat.engaged),
    quiet: seats.some((seat) => !seat.engaged),
    gap: gaps.length > 0,
  };
  const drawn = MARKS.filter((mark) => shown[mark]);
  return (
    <div className="dc-committee">
      <div className="dc-sides">
        <section className="dc-side" aria-labelledby={oursId}>
          <Eyebrow as="h3" size="xxsmall" id={oursId}>
            {t("deal.committee.ourTeam")}
          </Eyebrow>
          <ul className="dc-rows">
            {team.map((colleague) => (
              <li key={colleague.key} className="dc-row">
                <MarkDot mark="ours" />
                <span className="dc-row-text">
                  <span className="dc-row-name">{colleague.name}</span>
                  {colleague.jobs && (
                    <span className="dc-row-sub">{colleague.jobs}</span>
                  )}
                  {colleague.carries && (
                    <Badge tone="warning" wrap>
                      {t("deal.committee.carriesContact")}
                    </Badge>
                  )}
                </span>
              </li>
            ))}
          </ul>
          {teamUnread && (
            <p className="t-caption">{t("deal.committee.teamUnread")}</p>
          )}
          {!teamUnread && !teamPending && team.length === 0 && (
            <p className="t-caption">{t("assignments.empty")}</p>
          )}
        </section>
        <section className="dc-side" aria-labelledby={theirsId}>
          <Eyebrow as="h3" size="xxsmall" id={theirsId}>
            {t("deal.committee.theirTeam")}
          </Eyebrow>
          <SurfaceState
            loadingLabel={t("deal.committee.title")}
            state={state}
            emptyLabel={t("deal.standing.committee.none")}
          >
            <ul className="dc-rows">
              {seats.map((seat) => (
                <li key={seat.key} className="dc-row">
                  <button
                    type="button"
                    className="dc-seat-open"
                    onClick={() =>
                      navigate({ screen: "contacts", id: seat.key })
                    }
                  >
                    <MarkDot mark={seat.engaged ? "engaged" : "quiet"} />
                    <span className="dc-row-text">
                      <span className="dc-row-name">
                        {seat.name ?? t("coverage.seatWithheld")}
                      </span>
                      <span className="dc-row-sub">{seat.sub}</span>
                    </span>
                  </button>
                </li>
              ))}
              {gaps.map((gap) => (
                <GapRow key={gap.key} gap={gap} addSeat={addSeat} />
              ))}
            </ul>
          </SurfaceState>
        </section>
      </div>
      {drawn.length > 0 && (
        <ul className="dc-legend" aria-label={t("deal.committee.legend")}>
          {drawn.map((mark) => (
            <li key={mark}>
              <MarkDot mark={mark} />
              {t(MARK_WORDS[mark])}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function GapRow({
  gap,
  addSeat,
}: Readonly<{ gap: CommitteeGapRow; addSeat?: AddSeat }>) {
  const titleId = useId();
  return (
    <li className="dc-row dc-row-gap">
      <MarkDot mark="gap" />
      <span className="dc-row-text">
        <span className="dc-row-name" id={titleId}>
          {gap.title}
        </span>
        {addSeat?.(gap, titleId)}
      </span>
    </li>
  );
}

// The mark is the picture. The words beside it and the legend say what it
// means, so a reader of the text alone loses nothing.
function MarkDot({ mark }: Readonly<{ mark: Mark }>) {
  return <span className={`dc-dot dc-dot-${mark}`} aria-hidden="true" />;
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
