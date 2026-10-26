// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDate } from "./format";
import {
  FALLBACK_RECORD_ZONE,
  sameWallClockOnDay,
  startOfDayInZone,
  timezoneOptions,
  viewerZone,
  zoneNameAndOffset,
  zoneOffsetMs,
} from "./timezone";

afterEach(() => {
  vi.restoreAllMocks();
});

// The zone a runtime reports for itself, as viewerZone() asks for it.
function pretendReportedZone(timeZone: string): void {
  const real = Intl.DateTimeFormat().resolvedOptions();
  vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
    ...real,
    timeZone,
  });
}

describe("viewerZone", () => {
  it("answers with the zone the runtime reports", () => {
    pretendReportedZone("America/Los_Angeles");
    expect(viewerZone()).toBe("America/Los_Angeles");
  });

  it("falls back to UTC when the runtime reports no zone at all", () => {
    // A hardened or minimal runtime can leave `timeZone` empty, and an empty
    // string is not a zone Intl accepts — a caller handed it renders nothing
    // and throws mid-paint instead.
    pretendReportedZone("");
    expect(viewerZone()).toBe("UTC");
    expect(() =>
      formatDate("2026-07-02T00:30:00Z", "en", viewerZone()),
    ).not.toThrow();
  });

  it("answers freshly, so a reader who has moved is not read from a stale import", () => {
    pretendReportedZone("Asia/Tokyo");
    const first = viewerZone();
    vi.restoreAllMocks();
    pretendReportedZone("America/Los_Angeles");
    expect(first).toBe("Asia/Tokyo");
    expect(viewerZone()).toBe("America/Los_Angeles");
  });
});

describe("FALLBACK_RECORD_ZONE", () => {
  it("is a zone the formatters accept", () => {
    // format.ts refuses a fixed offset ("+01:00", "Etc/GMT-1") because those
    // freeze the DST rules. The fallback has to pass that door — it is what
    // renders when the installation's own zone cannot, so a fallback that
    // itself threw would turn a recoverable disagreement between two zone
    // databases into a blank page.
    expect(() =>
      formatDate("2026-07-02T00:30:00Z", "en", FALLBACK_RECORD_ZONE),
    ).not.toThrow();
  });
});

describe("a record's clock against the reader's", () => {
  it("reads an instant on the record's calendar, not the reader's", () => {
    // The distinction the whole module exists for, asserted against a zone
    // this test NAMES rather than against whatever the installation or the
    // fallback happens to hold. Berlin is a stand-in for any configured
    // record zone here; the claim is about the two purposes disagreeing, and
    // pinning it to a constant would make the test restate that constant
    // instead of the rule.
    //
    // 00:30 UTC is already 2 July in a zone east of UTC and still 1 July on
    // the US west coast — the boundary the two purposes disagree across.
    pretendReportedZone("America/Los_Angeles");
    const instant = "2026-07-02T00:30:00Z";
    expect(formatDate(instant, "en", "Europe/Berlin")).toBe("02/07/2026");
    expect(formatDate(instant, "en", viewerZone())).toBe("01/07/2026");
  });
});

describe("zoneOffsetMs", () => {
  it("reads the same offset however far into its second an instant is", () => {
    const twoHours = 2 * 60 * 60 * 1000;
    for (const at of ["2026-09-12T08:00:00.000Z", "2026-09-12T08:00:00.001Z"]) {
      expect(zoneOffsetMs(Date.parse(at), "Europe/Berlin")).toBe(twoHours);
    }
  });
});

describe("startOfDayInZone", () => {
  // A positive-offset zone's morning is still the PREVIOUS UTC calendar day,
  // which is exactly what a browser-local or UTC midnight gets wrong for a
  // timeline's date range.
  it("lands on the previous UTC day for a positive-offset zone", () => {
    expect(startOfDayInZone("2026-07-15", "Europe/Berlin")).toBe(
      "2026-07-14T22:00:00.000Z",
    );
  });

  it("shifts by whole calendar days for an exclusive range end, across a month edge", () => {
    expect(startOfDayInZone("2026-07-31", "America/New_York", 1)).toBe(
      "2026-08-01T04:00:00.000Z",
    );
  });

  // A day whose midnight does not exist. Santiago springs forward AT midnight
  // on 2026-09-06: its clock reads 23:00 on the 5th and then 01:00 on the 6th,
  // so `2026-09-06T00:00` is a wall-clock time that never happens there.
  //
  // Resolving it by offset arithmetic alone lands one hour before the jump —
  // an instant that still reads as the FIFTH. A "from 6 September" timeline
  // filter built on that pulls in an hour of the fifth, and the exclusive end
  // of a "to 5 September" range drops that same hour. Both are silent: the
  // rows are plausible, just off by an hour at one edge, once a year, in a
  // zone whoever wrote the filter probably does not live in.
  //
  // Reachable because the record zone is now the INSTALLATION's, not a
  // hardcoded Europe/Berlin — Berlin transitions at 02:00 and never removes
  // its own midnight.
  it("returns the day's first REAL instant when its midnight does not exist", () => {
    // 04:00Z is the transition itself — Santiago's clock jumps from 23:00 on
    // the 5th to 01:00 on the 6th at that instant, so it is the first moment
    // the zone reads as the 6th at all. 03:00Z, what offset arithmetic alone
    // resolves to, is still 23:00 on the 5th there.
    expect(startOfDayInZone("2026-09-06", "America/Santiago")).toBe(
      "2026-09-06T04:00:00.000Z",
    );
  });

  // The other side of the same zone's year, and what keeps the correction from
  // firing where it is not wanted: on a day whose midnight exists normally, the
  // resolved instant already reads as that day, so nothing is stepped forward.
  // Santiago is back on UTC-4 here, four hours from the spring-forward case
  // above, so a correction that ran unconditionally would show up as an offset
  // that is wrong by the transition's width rather than as a missing hour.
  it("leaves a day alone when its midnight exists", () => {
    expect(startOfDayInZone("2026-04-04", "America/Santiago")).toBe(
      "2026-04-04T03:00:00.000Z",
    );
  });
});

// Each row's answer differs from moving the UTC date by the same number of
// days, so a "swap the date" implementation fails it.
describe("sameWallClockOnDay", () => {
  it.each([
    {
      why: "a Bangkok 03:00 on the 16th moves to 03:00 on the 18th",
      source: "2026-01-15T20:00:00Z",
      zone: "Asia/Bangkok",
      day: "2026-01-18",
      moved: "2026-01-17T20:00:00.000Z",
    },
    {
      why: "a New York 22:00 on the 14th moves to 22:00 on the 20th",
      source: "2026-01-15T03:00:00Z",
      zone: "America/New_York",
      day: "2026-01-20",
      moved: "2026-01-21T03:00:00.000Z",
    },
    {
      why: "a Berlin 15:00 keeps 15:00 across the spring switch",
      source: "2026-03-20T14:00:00Z",
      zone: "Europe/Berlin",
      day: "2026-04-02",
      moved: "2026-04-02T13:00:00.000Z",
    },
    {
      why: "a Kathmandu 01:45 on the 16th moves to 01:45 on the 20th",
      source: "2026-01-15T20:00:00Z",
      zone: "Asia/Kathmandu",
      day: "2026-01-20",
      moved: "2026-01-19T20:00:00.000Z",
    },
  ])("$why", ({ source, zone, day, moved }) => {
    expect(sameWallClockOnDay(source, day, zone)).toBe(moved);
  });

  it("puts a wall time the switch removes at the first instant after the gap", () => {
    // 02:30 does not exist in Berlin on 29 March 2026: 02:00 jumps to 03:00.
    expect(
      sameWallClockOnDay("2026-03-20T01:30:00Z", "2026-03-29", "Europe/Berlin"),
    ).toBe("2026-03-29T01:00:00.000Z");
    // Lord Howe moves by half an hour, 02:00 to 02:30, on 4 October 2026.
    expect(
      sameWallClockOnDay(
        "2026-09-19T15:45:00Z",
        "2026-10-04",
        "Australia/Lord_Howe",
      ),
    ).toBe("2026-10-03T15:30:00.000Z");
  });

  it("refuses a day the zone skipped whole", () => {
    // Apia jumped from 29 to 31 December 2011; the 30th never happened there.
    expect(() =>
      sameWallClockOnDay("2011-12-29T22:00:00Z", "2011-12-30", "Pacific/Apia"),
    ).toThrow(RangeError);
    // The day after the jump exists, on the new +14:00 offset.
    expect(
      sameWallClockOnDay("2011-12-29T22:00:00Z", "2011-12-31", "Pacific/Apia"),
    ).toBe("2011-12-30T22:00:00.000Z");
  });

  it("takes the earlier of a wall time the switch repeats", () => {
    // 02:30 happens twice in Berlin on 25 October 2026: at 00:30Z on summer
    // time, then at 01:30Z on winter time.
    expect(
      sameWallClockOnDay("2026-10-20T00:30:00Z", "2026-10-25", "Europe/Berlin"),
    ).toBe("2026-10-25T00:30:00.000Z");
  });

  it("keeps the source's milliseconds", () => {
    expect(
      sameWallClockOnDay(
        "2026-01-15T14:00:00.250Z",
        "2026-01-16",
        "Asia/Bangkok",
      ),
    ).toBe("2026-01-16T14:00:00.250Z");
    // Also when the wall time falls in a gap and lands after it.
    expect(
      sameWallClockOnDay(
        "2026-03-20T01:30:00.250Z",
        "2026-03-29",
        "Europe/Berlin",
      ),
    ).toBe("2026-03-29T01:00:00.250Z");
  });

  it("refuses a year Date.UTC would move into the 1900s", () => {
    expect(() =>
      sameWallClockOnDay("2026-01-15T14:00:00Z", "0099-09-09", "Asia/Bangkok"),
    ).toThrow(RangeError);
  });

  it("refuses a day that does not exist", () => {
    expect(() =>
      sameWallClockOnDay("2026-01-15T14:00:00Z", "2026-02-30", "Asia/Bangkok"),
    ).toThrow(RangeError);
  });
});

describe("timezone choices", () => {
  it("names cities and their offset at the supplied date while retaining wire IDs", () => {
    const winter = timezoneOptions(
      "en",
      "Europe/Berlin",
      new Date("2026-01-15T12:00:00Z"),
    );
    const summer = timezoneOptions(
      "en",
      "Europe/Berlin",
      new Date("2026-07-15T12:00:00Z"),
    );
    expect(winter.find((zone) => zone.value === "Europe/Berlin")?.label).toBe(
      "Berlin (Europe) · UTC+1",
    );
    expect(summer.find((zone) => zone.value === "Europe/Berlin")?.label).toBe(
      "Berlin (Europe) · UTC+2",
    );
    expect(
      winter.find((zone) => zone.value === "Asia/Kathmandu")?.label ??
        winter.find((zone) => zone.value === "Asia/Katmandu")?.label,
    ).toContain("UTC+5:45");
  });

  it("keeps a saved alias even when it is absent from the browser's preferred names", () => {
    const zones = timezoneOptions(
      "en",
      "US/Eastern",
      new Date("2026-01-15T12:00:00Z"),
    );
    expect(zones.find((zone) => zone.value === "US/Eastern")?.label).toBe(
      "Eastern (US) · UTC-5",
    );
    expect(zones.filter((zone) => zone.value === "UTC")).toHaveLength(1);
  });
});

describe("zoneNameAndOffset", () => {
  const at = new Date("2026-09-05T10:30:00Z");

  it("names a given zone in the reader's language, not the IANA id", () => {
    expect(zoneNameAndOffset("en-GB", at, "Asia/Bangkok")).toBe(
      "Indochina Time · UTC+7",
    );
    expect(zoneNameAndOffset("vi-VN", at, "Asia/Bangkok")).toBe(
      "Giờ Đông Dương · UTC+7",
    );
  });

  it("reads the offset the zone has at that instant", () => {
    expect(zoneNameAndOffset("en-GB", at, "Europe/Berlin")).toBe(
      "Central European Summer Time · UTC+2",
    );
  });

  it("shows a zone the formatters refuse as it was stored", () => {
    expect(zoneNameAndOffset("en-GB", at, "Mars/Olympus")).toBe("Mars/Olympus");
    // Intl names this one, as "GMT+01:00"; the formatters refuse it.
    expect(zoneNameAndOffset("en-GB", at, "Etc/GMT-1")).toBe("Etc/GMT-1");
  });

  it("throws on a bad locale tag or instant instead of hiding it", () => {
    expect(() => zoneNameAndOffset("en_GB!", at, "Asia/Bangkok")).toThrow(
      RangeError,
    );
    expect(() =>
      zoneNameAndOffset("en-GB", new Date(Number.NaN), "Asia/Bangkok"),
    ).toThrow(RangeError);
  });
});
