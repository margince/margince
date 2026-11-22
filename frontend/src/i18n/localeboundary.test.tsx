/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  act,
  cleanup,
  type RenderResult,
  render,
  screen,
} from "@testing-library/react";
import { Suspense } from "react";
import { afterEach, expect, it, vi } from "vitest";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// A view behind the complete catalog, in a fresh module graph where `en` has
// loaded that far.
async function pageBehind(complete: boolean) {
  vi.resetModules();
  const { loadEssentialLocale, loadLocale } = await import("./catalogs");
  const { LocaleBoundary, LocaleProvider } = await import("./index");
  await (complete ? loadLocale("en") : loadEssentialLocale("en"));
  return (label: string) => (
    <LocaleProvider initial="en">
      <Suspense fallback={<p>Waiting</p>}>
        <LocaleBoundary level="complete">
          <p>{label}</p>
        </LocaleBoundary>
      </Suspense>
    </LocaleProvider>
  );
}

// Drawn once and then again, with the errors React reports in between.
async function drawTwice(complete: boolean) {
  const page = await pageBehind(complete);
  const errors = vi.spyOn(console, "error");
  let view: RenderResult | undefined;
  await act(async () => {
    view = render(page("First"));
  });
  expect(await screen.findByText("First")).toBeTruthy();
  await act(async () => {
    view?.rerender(page("Second"));
  });
  expect(screen.getByText("Second")).toBeTruthy();
  return errors;
}

it("draws a view whose catalog is loaded on the first render, without the fallback", async () => {
  const page = await pageBehind(true);
  render(page("First"));
  expect(screen.getByText("First")).toBeTruthy();
});

it("draws a view whose catalog is loaded, render after render, with no React error", async () => {
  expect(await drawTwice(true)).not.toHaveBeenCalled();
});

it("waits for a catalog still loading, then draws again with no React error", async () => {
  expect(await drawTwice(false)).not.toHaveBeenCalled();
});
