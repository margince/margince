// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  clockOrBlank,
  formatClockTime,
  parseTypedTime,
  timePattern,
  timePlaceholder,
} from "./timepattern";

const letters = { hour: "hh", minute: "mm" };

describe("timePattern", () => {
  it("follows the UI language's clock unless the installation names one", () => {
    expect(timePattern("en", "locale").twelveHours).toBe(false);
    expect(timePattern("en", "12h").twelveHours).toBe(true);
    expect(timePattern("de", "24h").twelveHours).toBe(false);
  });

  it("spells the halves of the day in the reader's language", () => {
    expect(timePattern("vi", "12h")).toMatchObject({
      morning: "SA",
      evening: "CH",
    });
  });
});

describe("formatClockTime", () => {
  it("shows the time on the reader's clock", () => {
    expect(formatClockTime("21:05", "de", timePattern("de", "24h"))).toBe(
      "21:05",
    );
    expect(formatClockTime("21:05", "en", timePattern("en", "12h"))).toBe(
      "09:05 pm",
    );
  });

  it("shows the end of a day that runs until midnight", () => {
    expect(formatClockTime("24:00", "en", timePattern("en", "24h"))).toBe(
      "24:00",
    );
    expect(formatClockTime("24:00", "en", timePattern("en", "12h"))).toBe(
      "12:00 am",
    );
  });
});

describe("parseTypedTime", () => {
  const day = timePattern("en", "24h");

  it.each([
    ["9", "09:00"],
    ["930", "09:30"],
    ["9:30", "09:30"],
    ["9.30", "09:30"],
    ["09h30", "09:30"],
    ["21:05", "21:05"],
    ["9:30 pm", "21:30"],
    ["12 am", "00:00"],
    ["12:15 P.M.", "12:15"],
  ])("reads %s as %s", (typed, time) => {
    expect(parseTypedTime(typed, day)).toBe(time);
  });

  it("reads the reader's own words for the halves of the day", () => {
    expect(parseTypedTime("9:30 CH", timePattern("vi", "12h"))).toBe("21:30");
  });

  it.each(["24:00", "9:60", "13 pm", "nine", "9:3"])("refuses %s", (typed) => {
    expect(parseTypedTime(typed, day)).toBeNull();
  });

  it("takes midnight as an end only when the field allows it", () => {
    expect(parseTypedTime("24:00", day, true)).toBe("24:00");
    expect(parseTypedTime("24", day, true)).toBe("24:00");
  });
});

describe("clockOrBlank", () => {
  it("keeps the minutes of a stored time and drops what is not one", () => {
    expect(clockOrBlank("09:30:00")).toBe("09:30");
    expect(clockOrBlank("24:00")).toBe("");
    expect(clockOrBlank("24:00", true)).toBe("24:00");
    expect(clockOrBlank(null)).toBe("");
  });
});

describe("timePlaceholder", () => {
  it("adds the morning word on a 12-hour clock", () => {
    expect(timePlaceholder(timePattern("en", "24h"), letters)).toBe("hh:mm");
    expect(timePlaceholder(timePattern("en", "12h"), letters)).toBe("hh:mm am");
  });
});
