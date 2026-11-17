// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  reportingCatalogClient,
  reportingLeadsEvaluation,
} from "../features/analytics/reporting.story-fixtures";
import { navigate } from "../lib/router";
import { StoryProviders } from "../testing/story-utils";
import { PaletteDataAnswer } from "./palettedata";

vi.mock("../lib/router", async (original) => ({
  ...(await original<typeof import("../lib/router")>()),
  navigate: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("shows the reporting record count when its money value is withheld", () => {
  const original = reportingLeadsEvaluation.metrics[0];
  if (!original) throw new Error("fixture needs a metric");
  const evaluation = {
    ...reportingLeadsEvaluation,
    metrics: [
      {
        ...original,
        value: 0,
        coverage: {
          ...original.coverage,
          status: "no_data" as const,
          withheld: true,
          eligible_count: 7,
        },
      },
    ],
  };
  const leave = vi.fn();
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <PaletteDataAnswer
        answer={{ outcome: "answered", evaluation, count: true }}
        onLeave={leave}
      />
    </StoryProviders>,
  );
  expect(screen.getByText("7")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Open in Analytics" }));
  expect(leave).toHaveBeenCalledOnce();
  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({ screen: "analytics" }),
    expect.any(Map),
  );
});
