// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { monthWindow, readMonth } from "./booking-guest-month";

const DAY = 86400000;

it("reads a month longer than the server's 31 days in two windows that meet", async () => {
  // October 2026 in Berlin ends an hour after 31 days: the clocks fall back.
  const window = monthWindow(
    new Date(2026, 9, 1),
    "Europe/Berlin",
    Date.parse("2026-09-01T00:00:00Z"),
  );
  if (!window) throw new Error("a month ahead has a window");
  const reads: [string, string][] = [];
  const late = { start: "2026-10-31T22:00:00Z", end: "2026-10-31T22:30:00Z" };

  const month = await readMonth(async (from, to) => {
    reads.push([from, to]);
    return { slots: from === window.from ? [] : [late], truncated: false };
  }, window);

  expect(Date.parse(window.to) - Date.parse(window.from)).toBeGreaterThan(
    31 * DAY,
  );
  expect(reads).toEqual([
    [window.from, "2026-10-31T22:00:00.000Z"],
    ["2026-10-31T22:00:00.000Z", window.to],
  ]);
  expect(month).toEqual({ slots: [late], truncated: false });
});

it("reads a month within the server's bound in one window", async () => {
  const window = monthWindow(
    new Date(2026, 10, 1),
    "Europe/Berlin",
    Date.parse("2026-09-01T00:00:00Z"),
  );
  if (!window) throw new Error("a month ahead has a window");
  const reads: [string, string][] = [];
  await readMonth(async (from, to) => {
    reads.push([from, to]);
    return { slots: [], truncated: false };
  }, window);
  expect(reads).toEqual([[window.from, window.to]]);
});
