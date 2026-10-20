// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { useQuery } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import { FirstRunNotice } from "./analytics.firstrun";
import { render as renderScreen, reportsStub } from "./analytics.testkit";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

// Says when the probe has answered, so a test of the notice's absence reads
// the settled answer and not the loading state.
function ProbeSettled() {
  const probe = useQuery({ queryKey: ["deals", "first-run"], enabled: false });
  return probe.isSuccess ? <span>probe settled</span> : null;
}

function draw(deals: unknown[]) {
  const asked: string[] = [];
  installFetchStub({
    "GET /me": meRoute({ deal: ["read"] }),
    "GET /deals": () =>
      jsonResponse({ data: deals, page: { next_cursor: null } }),
  });
  const stub = globalThis.fetch;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    asked.push(input instanceof Request ? input.url : String(input));
    return stub(input, init);
  }) as typeof fetch;
  render(
    <StoryProviders>
      <FirstRunNotice />
      <ProbeSettled />
    </StoryProviders>,
  );
  return asked;
}

it("names the first step when the reader can see no deal, archived ones included", async () => {
  const asked = draw([]);
  expect(
    await screen.findByText("No deals to report on yet"),
  ).toBeInTheDocument();
  expect(
    asked.some(
      (url) => url.includes("/deals") && url.includes("include_archived=true"),
    ),
  ).toBe(true);
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Open Deals" }));
  expect(window.location.hash).toMatch(/^#\/deals/);
});

it("says nothing once a deal exists", async () => {
  draw([{ id: "d-1", name: "First" }]);
  expect(await screen.findByText("probe settled")).toBeInTheDocument();
  expect(screen.queryByText("No deals to report on yet")).toBeNull();
});

it("shows on the deal sections and not on Delivery", async () => {
  vi.stubGlobal("fetch", reportsStub({ dealRead: true }));
  window.location.hash = "#/analytics/pipeline";
  const view = renderScreen(<AnalyticsScreen />);
  expect(
    await screen.findByText("No deals to report on yet"),
  ).toBeInTheDocument();
  view.unmount();

  window.location.hash = "#/analytics/delivery";
  const asked: string[] = [];
  const reported: string[] = [];
  vi.stubGlobal(
    "fetch",
    reportsStub({ dealRead: true, onRun: (key) => reported.push(key) }),
  );
  const stub = globalThis.fetch;
  vi.stubGlobal("fetch", ((input: RequestInfo | URL, init?: RequestInit) => {
    asked.push(input instanceof Request ? input.url : String(input));
    return stub(input, init);
  }) as typeof fetch);
  renderScreen(<AnalyticsScreen />);
  // Delivery's three cards have asked for their reports, so the screen has
  // mounted everything it will; a notice there would have probed by now.
  await vi.waitFor(() => expect(reported).toContain("projects-by-phase"));
  expect(asked.some((url) => /\/deals(\?|$)/.test(url))).toBe(false);
  expect(screen.queryByText("No deals to report on yet")).toBeNull();
  vi.unstubAllGlobals();
});
