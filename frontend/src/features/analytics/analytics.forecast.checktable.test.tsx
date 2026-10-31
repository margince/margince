/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { meFixture } from "../../testing/mefixture";
import { ReviewActionsProvider } from "../../testing/reviewactions";
import { ForecastReview } from "./analytics.forecast.review";

// The review as a door from Sales review opens it. It opens on one finding or
// one group, and offers only the answers this reader may give.

type InputCheck = components["schemas"]["InputCheck"];

function check(
  id: string,
  label: string,
  type: string,
  mode: "answer" | "read" = "answer",
): InputCheck {
  return {
    id,
    type,
    subject_kind: "deal",
    subject_id: `d-${id}`,
    subject: { type: "deal", id: `d-${id}`, label },
    severity: "high",
    status: "open",
    first_seen_at: "2026-10-12T02:00:00Z",
    last_seen_at: "2026-10-31T02:00:00Z",
    inputs_changed: false,
    destination: { screen: "forecast_review", mode },
  };
}

const HARBOUR = check("f-1", "Harbourline depot retrofit", "close_past");
const QUAYSIDE = check("f-2", "Quayside fleet tools", "no_next_step");
const MARLOW = check("f-3", "Marlow workshop suite", "close_past");

function show(address: string, findings: InputCheck[]) {
  window.location.hash = address;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      if (url.endsWith("/me"))
        return jsonResponse(
          meFixture({
            allow: { deal: ["read", "update"], forecast: ["read", "update"] },
          }),
        );
      if (url.includes("/forecast/assurance/exceptions"))
        return jsonResponse({ data: findings });
      if (url.includes("/forecast/assurance"))
        return jsonResponse({ status: "complete", readiness: "needs_review" });
      return jsonResponse({}, 404);
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ReviewActionsProvider>
          <ForecastReview />
        </ReviewActionsProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("a door from Sales review", () => {
  it("opens on the finding it named and gives it focus", async () => {
    show("#/analytics/forecast?finding=f-3", [HARBOUR, QUAYSIDE, MARLOW]);
    await screen.findByRole("link", { name: "Marlow workshop suite" });
    await waitFor(() =>
      expect(document.activeElement?.getAttribute("data-finding")).toBe("f-3"),
    );
  });

  it("says a finding answered since the link was made is no longer open", async () => {
    show("#/analytics/forecast?finding=f-9", [HARBOUR]);
    expect(
      await screen.findByText(
        "This finding is no longer open. It was answered or withdrawn after the link was made.",
      ),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Answer" })).toBeVisible();
  });

  it("lists one group's findings, and shows every finding again on request", async () => {
    show("#/analytics/forecast?check=close_past", [HARBOUR, QUAYSIDE, MARLOW]);
    expect(
      await screen.findByRole("link", { name: "Harbourline depot retrofit" }),
    ).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Marlow workshop suite" }),
    ).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "Quayside fleet tools" }),
    ).toBeNull();
    const user = userEvent.setup({ delay: null });
    await user.click(
      screen.getByRole("button", { name: "Show every finding" }),
    );
    expect(
      await screen.findByRole("link", { name: "Quayside fleet tools" }),
    ).toBeVisible();
  });

  it("offers no answer on a finding this reader may only read", async () => {
    show("#/analytics/forecast?finding=f-1", [
      check("f-1", "Harbourline depot retrofit", "close_past", "read"),
    ]);
    await screen.findByRole("link", { name: "Harbourline depot retrofit" });
    expect(screen.queryByRole("button", { name: "Answer" })).toBeNull();
    expect(screen.getByText("Read only")).toBeVisible();
    await waitFor(() =>
      expect(document.activeElement?.getAttribute("data-finding")).toBe("f-1"),
    );
  });
});
