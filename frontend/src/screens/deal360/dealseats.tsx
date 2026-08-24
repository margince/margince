// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who is on this deal, in the rail.
//
// The seats used to sit in the coverage card in the main column, under the
// findings about them. That put the same two facts on screen twice once the
// readings band started counting them and Deal360 started naming the risks as
// chips: a reader met "Single-threaded" in the band, again as a chip, and a
// third time as a card. The seats are CONTEXT — who these people are — so they
// belong where the company page keeps its people, in the rail.
//
// The rows are the coverage card's own markup, moved rather than rewritten:
// `.net-seats` already spells a seat with its name, whether they are engaged
// and their role, and a second spelling of a person-with-role row is exactly
// the drift the frontend rulebook names.

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { PanelBody } from "../../design-system/panel";
import { sectionState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { dealRoleLabel, RailPanel } from "../record360";
import "../network.css";

type DealCoverage = components["schemas"]["DealCoverage"];

/**
 * DealSeats lists the stakeholders and the colleagues carrying the deal.
 *
 * Withheld and empty are different answers and the panel says which: a caller
 * without the relationship grant is served no seats, and drawing that as "no
 * stakeholders" would report an absence the server never claimed.
 */
export function DealSeats({
  coverage,
  withheld,
  pending,
}: Readonly<{
  coverage?: DealCoverage;
  withheld: boolean;
  pending: boolean;
}>) {
  const t = useT();
  const seats = coverage?.stakeholders ?? [];
  const ours = coverage?.our_side ?? [];
  const state = sectionState(
    withheld
      ? { sections_omitted: ["stakeholders"] }
      : { sections_omitted: [] },
    "stakeholders",
    Boolean(coverage),
    seats.length,
    pending,
  );
  return (
    <RailPanel
      title={t("deal.seats.title")}
      state={state}
      emptyLabel={t("deal.seats.empty")}
      footer={
        ours.length > 0 ? (
          <p className="t-caption">
            {t("deal.seats.ours", { count: ours.length })}
          </p>
        ) : undefined
      }
    >
      <PanelBody>
        <ul className="net-seats">
          {seats.map((seat) => (
            <li key={seat.person_id}>
              <span className="net-seat-name">
                {/* Absent when the caller may not read that person: the seat
                    still counts toward coverage, so it is shown, and only the
                    identity is withheld. */}
                {seat.person_name ?? t("coverage.seatWithheld")}
              </span>
              <Badge tone={seat.engaged ? "success" : undefined}>
                {seat.engaged ? t("coverage.engaged") : t("coverage.quiet")}
              </Badge>
              <span className="t-caption">{dealRoleLabel(seat.role, t)}</span>
            </li>
          ))}
        </ul>
      </PanelBody>
    </RailPanel>
  );
}
