// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { DealCockpit } from "./dealcockpit";

// The band a reader scans before reading anything else: where this deal
// stands in the pipeline. What used to be three readings beside the ladder
// now lives on the head's facts strip, in the brief, and on the Deal Room
// tab, so the ladder is the whole of what this band draws.

type Deal = components["schemas"]["Deal"];

function deal(over: Partial<Deal> = {}): Deal {
  return {
    id: "01a03000-0000-7000-8000-000000000001",
    name: "Fleet telematics rollout",
    amount_minor: 4_500_000,
    currency: "EUR",
    status: "open",
    stalled: false,
    source: "manual",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  } as Deal;
}

function show(over: Partial<Deal> = {}, advanceRefused = false) {
  return render(
    <LocaleProvider initial="en">
      <DealCockpit
        deal={deal(over)}
        stages={[]}
        advancing={false}
        advanceRefused={advanceRefused}
        onAdvance={() => {}}
      />
    </LocaleProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the cockpit band", () => {
  it("draws the stage ladder under its own landmark", () => {
    show();
    expect(
      screen.getByRole("region", { name: en["deal.strip.title"] }),
    ).toBeInTheDocument();
  });

  it("carries the ladder's own refusal on a deal that takes no move", () => {
    show({}, true);
    expect(screen.getByText(en["deal.closedTakesNoStage"])).toBeInTheDocument();
  });
});
