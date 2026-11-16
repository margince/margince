/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// Each case loads the module afresh, because whether the first screen has
// settled is decided once per document.
async function freshMarks(hash: string) {
  vi.resetModules();
  window.location.hash = hash;
  return import("./journeymarks");
}

describe("the shell's side reads", () => {
  afterEach(() => {
    vi.useRealTimers();
    window.location.hash = "";
  });

  it.each(["home:useful", "home:failed"] as const)(
    "wait on Home until it marks %s",
    async (mark) => {
      vi.useFakeTimers();
      const marks = await freshMarks("#/home");
      const shell = renderHook(() => marks.useFirstScreenSettled());
      const other = renderHook(() => marks.useFirstScreenSettled());
      act(() => marks.markJourney("session"));
      expect(shell.result.current).toBe(false);
      act(() => marks.markJourney(mark));
      expect(shell.result.current).toBe(true);
      expect(other.result.current).toBe(true);
    },
  );

  it("wait on Home no longer than the grace", async () => {
    vi.useFakeTimers();
    const marks = await freshMarks("#/home");
    const shell = renderHook(() => marks.useFirstScreenSettled());
    act(() => vi.advanceTimersByTime(marks.FIRST_SCREEN_GRACE_MS - 1));
    expect(shell.result.current).toBe(false);
    act(() => vi.advanceTimersByTime(1));
    expect(shell.result.current).toBe(true);
  });

  it("start at once on any other first screen", async () => {
    const marks = await freshMarks("#/deals");
    expect(renderHook(() => marks.useFirstScreenSettled()).result.current).toBe(
      true,
    );
  });
});
