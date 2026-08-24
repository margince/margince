// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDate } from "./format";
import { FALLBACK_RECORD_ZONE, startOfDayInZone, viewerZone } from "./timezone";

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
