// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { FIXTURE_TIMEZONE } from "../testing/mefixture";
import { reminderAtWallClock, reminderChoices } from "./taskreminder.choices";

const ZONE = FIXTURE_TIMEZONE;
// 14:00 in Berlin on 3 March 2026, winter time (UTC+1).
const NOW = Date.parse("2026-03-03T13:00:00.000Z");

describe("reminderChoices", () => {
  it("offers an hour from now and tomorrow at 09:00 on the reader's clock", () => {
    const [inAnHour, morning] = reminderChoices(NOW, ZONE, {});
    expect(inAnHour).toEqual({
      key: "inAnHour",
      write: { remind_at: "2026-03-03T14:00:00.000Z" },
    });
    expect(morning?.key).toBe("tomorrowMorning");
    expect(morning?.write).toEqual({
      remind_at: "2026-03-04T08:00:00.000Z",
      remind_intent: { wall_clock: "2026-03-04T09:00", timezone: ZONE },
    });
  });

  it("offers an hour before a due time of day that is still more than an hour away", () => {
    const choices = reminderChoices(NOW, ZONE, {
      at: "2026-03-03T16:30:00.000Z",
      hasTime: true,
    });
    expect(choices.map((choice) => choice.key)).toEqual([
      "inAnHour",
      "tomorrowMorning",
      "beforeDue",
    ]);
    expect(choices[2]?.write).toEqual({
      remind_at: "2026-03-03T15:30:00.000Z",
    });
  });

  it("leaves out before-due for a day-only deadline, which has no hour", () => {
    const keys = reminderChoices(NOW, ZONE, {
      at: "2026-03-05T22:59:59.000Z",
      hasTime: false,
    }).map((choice) => choice.key);
    expect(keys).not.toContain("beforeDue");
  });

  it("leaves out before-due when that hour has already begun", () => {
    const keys = reminderChoices(NOW, ZONE, {
      at: "2026-03-03T13:45:00.000Z",
      hasTime: true,
    }).map((choice) => choice.key);
    expect(keys).not.toContain("beforeDue");
  });
});

describe("reminderAtWallClock", () => {
  it("keeps the wall clock across a summer-time change", () => {
    expect(reminderAtWallClock("2026-07-01", "09:30", ZONE)).toEqual({
      remind_at: "2026-07-01T07:30:00.000Z",
      remind_intent: { wall_clock: "2026-07-01T09:30", timezone: ZONE },
    });
  });
});
