// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it } from "vitest";
import { formatDue, formatDueCompact } from "./format.dates";
import { setDateTimePreferences } from "./preferences";

afterEach(() => {
  setDateTimePreferences({ dateFormat: "locale", timeFormat: "locale" });
});

it("shows the chosen clock and names its zone to a reader elsewhere", () => {
  setDateTimePreferences({ dateFormat: "ymd", timeFormat: "24h" });
  const intent = {
    wall_clock: "2027-03-28T02:30:00",
    timezone: "Europe/Berlin",
  };
  const resolved = "2027-03-28T01:00:00Z";
  for (const format of [formatDue, formatDueCompact]) {
    const ownZone = format(resolved, true, "en", intent.timezone, intent);
    expect(ownZone).toContain("02:30");
    expect(format(resolved, true, "en", "Pacific/Honolulu", intent)).toBe(
      `${ownZone} (Europe/Berlin)`,
    );
  }
});

it("keeps a chosen calendar day when the reader is still on the previous day", () => {
  setDateTimePreferences({ dateFormat: "ymd", timeFormat: "24h" });
  const intent = { wall_clock: "2027-03-28T23:59:59", timezone: "Asia/Tokyo" };
  expect(
    formatDue("2027-03-28T14:59:59Z", false, "en", "Pacific/Honolulu", intent),
  ).toBe("2027-03-28 (Asia/Tokyo)");
});

it("continues to render a legacy instant in the reader's zone", () => {
  setDateTimePreferences({ dateFormat: "ymd", timeFormat: "24h" });
  expect(
    formatDue("2027-03-28T01:00:00Z", false, "en", "Pacific/Honolulu"),
  ).toBe("2027-03-27");
});
