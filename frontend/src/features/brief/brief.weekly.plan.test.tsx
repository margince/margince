/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function stubWeeks() {
  stubApi({
    "GET /weekly-reviews/latest": () => jsonResponse(null, 404),
    "GET /weekly-reviews": () =>
      jsonResponse({ weeks: ["2026-06-29", "2026-06-22"] }),
  });
}

it("draws the live plan outside the frozen report, titled by its own dates", async () => {
  window.location.hash = "#/home?view=weekly";
  stubWeeks();
  render(<BriefScreen />);
  const plan = await screen.findByRole("heading", { name: /^Plan · / });
  expect(plan.textContent).toMatch(/8.*14 June/);
  expect(plan.closest("#brief-weekly")).toBeNull();
});

it("keeps the current plan behind its opener while an earlier week is shown", async () => {
  window.location.hash = "#/home?view=weekly&week=2026-06-22";
  stubWeeks();
  render(<BriefScreen />);
  const opener = await screen.findByText(en["plan.titleThisWeek"]);
  const details = opener.closest("details");
  expect(details).toBeTruthy();
  expect(details?.open).toBe(false);
  expect(details?.closest("#brief-weekly")).toBeNull();
});
