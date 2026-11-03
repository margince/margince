// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { UTC_ZONE } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { largestStageTotal } from "./boardtotals";
import { type BoardMoneyColumn, PipelineBoard } from "./pipelineboard";

afterEach(cleanup);

function stage(
  id: string,
  rawMinor: number | null,
  more: Partial<BoardMoneyColumn> = {},
): BoardMoneyColumn {
  return {
    stage: id,
    label: id,
    probabilityPct: 50,
    rawMinor,
    weightedMinor: rawMinor == null ? null : rawMinor / 2,
    currency: "EUR",
    deals: [],
    ...more,
  };
}

function board(columns: BoardMoneyColumn[]) {
  return render(
    <LocaleProvider initial="en">
      <PipelineBoard
        columns={columns}
        cardHref={(deal) => `#/deals/${deal.id}`}
        zone={UTC_ZONE}
      />
    </LocaleProvider>,
  );
}

function fills(container: HTMLElement): string[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>(".meterbar > span"),
  ).map((fill) => fill.style.width);
}

describe("the stage totals at the head of a board column", () => {
  it("draws each stage's bar against the largest stage on the board", () => {
    const { container } = board([stage("a", 400_000), stage("b", 100_000)]);
    expect(fills(container)).toEqual(["100%", "25%"]);
  });

  it("fills the weighted share inside the stage's own bar", () => {
    const { container } = board([
      stage("a", 400_000, { weightedMinor: 100_000 }),
    ]);
    const part = container.querySelector<HTMLElement>(".meterbar-part");
    expect(part?.style.width).toBe("25%");
  });

  // A bar is a length on one scale, and euros and dollars share none.
  it("draws no bars when the stage totals name two currencies", () => {
    const columns = [stage("a", 400_000), stage("b", 100_000)];
    columns[1] = { ...columns[1], currency: "USD" };
    expect(largestStageTotal(columns)).toBe(0);
    expect(fills(board(columns).container)).toEqual([]);
  });

  it("leaves a withheld or absent total out of the scale", () => {
    const columns = [
      stage("a", 900_000, { sumHidden: true }),
      stage("b", null),
      stage("c", 200_000),
    ];
    expect(largestStageTotal(columns)).toBe(200_000);
  });

  // The figures are rounded, so the exact amounts must be one focus away.
  it("spells out the exact amounts and the notes in the total's tip", () => {
    board([
      stage("a", 106_254_800, {
        weightedMinor: 10_625_480,
        probabilityPct: 10,
        unpricedCount: 6,
        asOf: "2026-11-03T08:00:00Z",
      }),
    ]);
    const total = screen.getByText(/^€1\.06m$/i);
    expect(total.tabIndex).toBe(0);
    fireEvent.focus(total);
    const tip = screen.getByRole("tooltip").textContent ?? "";
    expect(tip).toContain("€1,062,548.00 open, €106,254.80 weighted at 10%.");
    expect(tip).toContain(
      "6 deals have no price and are left out of both totals.",
    );
    expect(tip).toContain("Converted at rates as of");
  });
});
