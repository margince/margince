/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Suspense } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  CardBoundary,
  SilentCardBoundary,
} from "../design-system/cardboundary";
import { reloadForNewBuild } from "../lib/stalebuild";
import { LocaleProvider } from "./index";
import { lazy } from "./lazy";

beforeEach(() => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-09-22T12:00:00Z"));
  sessionStorage.clear();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("keeps the page usable after an optional chunk fails and reloads on Retry", async () => {
  const reload = vi.fn();
  vi.spyOn(window, "location", "get").mockReturnValue({
    ...window.location,
    reload,
  });
  expect(reloadForNewBuild()).toBe(true);
  const Panel = lazy(
    async () => {
      throw new TypeError(
        "Failed to fetch dynamically imported module: /assets/panel.js",
      );
    },
    "essential",
    CardBoundary,
  );
  render(
    <LocaleProvider initial="en" initialEssential>
      <p>Opening figures</p>
      <Suspense fallback={<p>Waiting for panel</p>}>
        <Panel />
      </Suspense>
    </LocaleProvider>,
  );
  const retry = await screen.findByRole("button", { name: "Retry" });
  expect(screen.getByText("Opening figures")).toBeTruthy();
  expect(document.body.textContent).not.toContain("/assets/panel.js");
  await userEvent.setup().click(retry);
  expect(reload).toHaveBeenCalledTimes(2);
});

it("omits failed optional chrome while leaving its page and navigation usable", async () => {
  const Chrome = lazy(
    async () => {
      throw new TypeError(
        "Failed to fetch dynamically imported module: /assets/chrome.js",
      );
    },
    "essential",
    SilentCardBoundary,
  );
  render(
    <LocaleProvider initial="en" initialEssential>
      <a href="#/deals">Deals</a>
      <p>Opening figures</p>
      <Suspense fallback={<p>Waiting for chrome</p>}>
        <Chrome />
      </Suspense>
    </LocaleProvider>,
  );
  await waitFor(() =>
    expect(screen.queryByText("Waiting for chrome")).toBeNull(),
  );
  expect(screen.getByRole("link", { name: "Deals" })).toBeTruthy();
  expect(screen.getByText("Opening figures")).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
});
