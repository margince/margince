// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { forgetSeat } from "../../lib/storage";
import {
  openOverview as open,
  prospectingEvaluation,
  salesEvaluation,
} from "./analytics.overview.testkit";

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

const PROMPT = "Choose a start and end date to view this period.";

describe("A custom interval with no days yet", () => {
  it.each([
    ["Sales review", "sales", salesEvaluation],
    ["My prospecting", "prospecting", prospectingEvaluation],
  ])(
    "%s says what to choose and shows no loading bars",
    async (_name, overview, evaluation) => {
      open(
        `#/analytics/overview?overview=${overview}&period=custom`,
        evaluation,
        { template: "sdr" },
      );
      expect(await screen.findByText(PROMPT)).toBeVisible();
      await waitFor(() =>
        expect(document.querySelector('[aria-busy="true"]')).toBeNull(),
      );
    },
  );

  it.each([
    ["31/02/2026", /There is no 31\/02\/2026 in the calendar/],
    ["abc", /Enter a date as/],
  ])(
    "tells a typed %s, and keeps asking for both days",
    async (typed, refusal) => {
      const user = userEvent.setup({ delay: null });
      open(
        "#/analytics/overview?overview=sales&period=custom",
        salesEvaluation,
        {
          template: "sdr",
        },
      );
      await user.type(await screen.findByLabelText("From"), typed);
      await user.tab();
      expect(await screen.findByText(refusal)).toBeVisible();
      expect(screen.getByText(PROMPT)).toBeVisible();
    },
  );

  it("tells a bad day typed over a valid one, and asks for the day again", async () => {
    const user = userEvent.setup({ delay: null });
    open(
      "#/analytics/overview?overview=sales&period=custom&from=2026-10-01&through=2026-10-15",
      salesEvaluation,
      { template: "sdr" },
    );
    const from = await screen.findByLabelText("From");
    await user.clear(from);
    await user.type(from, "31/02/2026");
    await user.tab();
    expect(await screen.findByText(/There is no 31\/02\/2026/)).toBeVisible();
    expect(screen.getByText(PROMPT)).toBeVisible();
  });
});
