/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { DealCommercial } from "./dealcommercial";

// What the commercial panel says about recurring revenue.
//
// The cases that matter are the ones where a figure could be shown wrong
// rather than not at all: a withheld currency, a currency whose minor unit is
// not two digits, and a monthly division that did not come out even.

type Deal = components["schemas"]["Deal"];

const deal = (over: Partial<Deal>): Deal =>
  ({
    id: "d1",
    name: "Seasonal payroll",
    status: "open",
    source: "manual",
    captured_by: "u1",
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    ...over,
  }) as Deal;

const show = (d: Deal) =>
  render(
    <StoryProviders>
      <DealCommercial deal={d} sources={[]} />
    </StoryProviders>,
  );

describe("DealCommercial recurring revenue", () => {
  it("shows the annual figure and an exact monthly reading", () => {
    show(deal({ expected_arr_minor: 1_200_000, currency: "EUR" }));
    expect(screen.getByText(/12,000\.00/)).toBeTruthy();
    expect(screen.getByText(/1,000\.00/)).toBeTruthy();
    // Nothing was lost, so nothing is hedged.
    expect(screen.queryByText(/approx/i)).toBeNull();
  });

  it("marks the monthly reading when the division loses something", () => {
    show(deal({ expected_arr_minor: 10_000, currency: "EUR" }));
    expect(screen.getByText(/approx/i)).toBeTruthy();
    // 100.00 a year is 8.33 a month, rounded half up from 8.333…
    expect(screen.getByText(/8\.33/)).toBeTruthy();
  });

  it("scales by the currency's own minor unit, not by a hundred", () => {
    // JPY carries no minor unit, so 1,000,000 minor units is ¥1,000,000 —
    // shown as ¥10,000.00 if anything divided by a hundred.
    show(deal({ expected_arr_minor: 1_000_000, currency: "JPY" }));
    expect(screen.getByText(/1,000,000/)).toBeTruthy();
  });

  it("renders no money at all when the currency is withheld", () => {
    // The read mask withholds the money reading as one unit, so an ARR with no
    // code is a field the caller may not see — not a figure to guess a unit for.
    const { container } = show(deal({ expected_arr_minor: 1_200_000 }));
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing when nobody has recorded any commercial context", () => {
    const { container } = show(deal({}));
    expect(container.innerHTML).toBe("");
  });
});
