/** @vitest-environment jsdom */
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { translate } from "../i18n";
import { formatCountdown, formatElapsed, useNow } from "./now";

// Task 9 (AC-7 groundwork): useNow/formatCountdown feed Task 10's live
// expiry countdown. No real clock in tests (craft T11) — vi.useFakeTimers()
// pins Date.now() and drives setInterval deterministically.

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

describe("formatCountdown (pure)", () => {
  it("renders minutes and seconds for a positive remainder", () => {
    expect(formatCountdown(90_000, t, "en")).toBe("1m 30s");
  });

  it("rolls up to hours and days rather than carrying minutes as the top unit", () => {
    const minute = 60_000;
    const hour = 60 * minute;
    const day = 24 * hour;

    // The approvals inbox stages drafts with a multi-day TTL. Carried as
    // minutes, three days read "4320m 0s" — a number nobody converts on sight.
    expect(formatCountdown(3 * day, t, "en")).toBe("3d 0h");
    expect(formatCountdown(3 * day + 4 * hour + 59 * minute, t, "en")).toBe(
      "3d 4h",
    );
    expect(formatCountdown(2 * hour + 15 * minute + 30_000, t, "en")).toBe(
      "2h 15m",
    );

    // The boundaries either side of each rollover.
    expect(formatCountdown(day, t, "en")).toBe("1d 0h");
    expect(formatCountdown(day - 1, t, "en")).toBe("23h 59m");
    expect(formatCountdown(hour, t, "en")).toBe("1h 0m");
    expect(formatCountdown(hour - 1, t, "en")).toBe("59m 59s");
  });

  it("renders the expired sentinel for zero or negative remainders", () => {
    expect(formatCountdown(0, t, "en")).toBe("Expired");
    expect(formatCountdown(-1, t, "en")).toBe("Expired");
  });
});

describe("useNow (interval clock)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("advances as fake time is advanced by the interval", () => {
    vi.setSystemTime(0);
    const { result, unmount } = renderHook(() => useNow(1000));
    expect(result.current).toBe(0);

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current).toBe(1000);

    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(result.current).toBe(4000);

    unmount();
  });
});

// formatElapsed is formatCountdown's other direction — how long AGO — and it
// drives the AI page's "last call" line. Every boundary is pinned because the
// unit it picks is the whole answer: a span that reads "1 h ago" when it is a
// day old tells an operator the runtime is healthy.
describe("formatElapsed (pure)", () => {
  it("floors under a minute to just now", () => {
    expect(formatElapsed(0, t, "en")).toBe("just now");
    expect(formatElapsed(59_999, t, "en")).toBe("just now");
  });

  it("crosses into minutes at exactly one", () => {
    expect(formatElapsed(60_000, t, "en")).toBe("1 min ago");
    expect(formatElapsed(59 * 60_000, t, "en")).toBe("59 min ago");
  });

  it("crosses into hours at exactly one", () => {
    expect(formatElapsed(60 * 60_000, t, "en")).toBe("1 h ago");
    expect(formatElapsed(23 * 60 * 60_000, t, "en")).toBe("23 h ago");
  });

  it("crosses into days at exactly one", () => {
    expect(formatElapsed(24 * 60 * 60_000, t, "en")).toBe("1 d ago");
    expect(formatElapsed(9 * 24 * 60 * 60_000, t, "en")).toBe("9 d ago");
  });

  // Two machines disagreeing about the clock, not an event in the future. A
  // negative age would read as one, so the floor holds on both sides of zero.
  it("reads a stamp from the future as just now rather than a negative age", () => {
    expect(formatElapsed(-1, t, "en")).toBe("just now");
    expect(formatElapsed(-90_000, t, "en")).toBe("just now");
  });

  // One unit, never two: this is read to decide whether a figure is current,
  // and the second unit buys nothing for that question.
  it("names one unit only", () => {
    expect(formatElapsed(25 * 60 * 60_000 + 42 * 60_000, t, "en")).toBe(
      "1 d ago",
    );
  });
});
