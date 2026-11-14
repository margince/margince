// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */

import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// Each case loads the module afresh, because whether the first screen has
// settled is decided once per document.
async function freshMarks(hash: string) {
  vi.resetModules();
  window.location.hash = hash;
  return import("./journeymarks");
}

function settled(marks: typeof import("./journeymarks")): boolean {
  return renderHook(() => marks.useFirstScreenSettled()).result.current;
}

describe("the shell's side reads", () => {
  afterEach(() => {
    vi.useRealTimers();
    window.location.hash = "";
  });

  it("wait on Home until its day is on screen", async () => {
    vi.useFakeTimers();
    const marks = await freshMarks("#/home");
    marks.markJourney("session");
    expect(settled(marks)).toBe(false);
    marks.markJourney("home:useful");
    expect(settled(marks)).toBe(true);
  });

  it("wait on Home no longer than the grace", async () => {
    vi.useFakeTimers();
    const marks = await freshMarks("#/home");
    vi.advanceTimersByTime(marks.FIRST_SCREEN_GRACE_MS - 1);
    expect(settled(marks)).toBe(false);
    vi.advanceTimersByTime(1);
    expect(settled(marks)).toBe(true);
  });

  it("start at once on any other first screen", async () => {
    const marks = await freshMarks("#/deals");
    expect(settled(marks)).toBe(true);
  });
});
