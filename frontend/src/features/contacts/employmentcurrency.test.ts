import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import currencyCases from "../../../../backend/internal/shared/kernel/employment/testdata/currency.json";
import type { components } from "../../api/schema";
import {
  currentEmployer,
  formerEmployers,
  stillHeld,
  today,
} from "../../lib/employmentcurrency";

type Employment = components["schemas"]["Contact360Employment"];

// The client twin of contacts.EmploymentIsCurrentSQL. It exists because the flag is
// written once and never rewritten, so a screen that trusted it would name a
// company the server's own contact count had already stopped counting — and it is
// tested directly because the boundary is one day wide and a rendering test would
// only ever exercise whichever side of it the fixture happened to sit on.
//
// The clock is PINNED. These assertions are about a date comparison, so a real
// clock would make them pass or fail depending on the hour the suite ran, which
// is the flake this module was written to prevent elsewhere.
const NOON = new Date("2026-08-19T12:00:00Z");
const ZONE = "UTC";

function employment(over: Partial<Employment>): Employment {
  return {
    relationship_id: "rel-1",
    company_id: "o-1",
    is_current_primary: false,
    ended_at: null,
    ...over,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOON);
});

afterEach(() => {
  vi.useRealTimers();
});

describe("today", () => {
  it("is the zone's day, zero-padded, in ISO order", () => {
    // Order and padding both matter: the value is compared lexicographically
    // below and sent to the server as a `date`.
    expect(today(ZONE)).toBe("2026-08-19");
  });

  it("pads a single-digit month and day", () => {
    vi.setSystemTime(new Date("2026-01-02T23:30:00Z"));
    expect(today(ZONE)).toBe("2026-01-02");
  });
});

describe("a departure day is judged in the installation zone", () => {
  // 23:30 on 2026-08-19 in Los Angeles is already 06:30 on the 20th in UTC and
  // 18:30 on the 20th in Auckland: three different days for one instant.
  const instant = new Date("2026-08-20T06:30:00Z");
  const endsToday = (zone: string) => ({ ended_at: today(zone) });

  it("reads today in the zone it is given, not the browser's", () => {
    vi.setSystemTime(instant);
    expect(today("America/Los_Angeles")).toBe("2026-08-19");
    expect(today("UTC")).toBe("2026-08-20");
    expect(today("Pacific/Auckland")).toBe("2026-08-20");
  });

  it("west of UTC, a job ending on the zone's day is over and the next day's is not", () => {
    vi.setSystemTime(instant);
    const zone = "America/Los_Angeles";
    expect(stillHeld(employment(endsToday(zone)), zone)).toBe(false);
    expect(stillHeld(employment({ ended_at: "2026-08-20" }), zone)).toBe(true);
  });

  it("east of UTC, a job ending on the UTC day is already over", () => {
    vi.setSystemTime(new Date("2026-08-19T18:30:00Z"));
    const zone = "Pacific/Auckland";
    expect(today(zone)).toBe("2026-08-20");
    expect(stillHeld(employment({ ended_at: "2026-08-20" }), zone)).toBe(false);
    expect(stillHeld(employment({ ended_at: "2026-08-20" }), "UTC")).toBe(true);
  });
});

describe("stillHeld", () => {
  it("holds a job with no end date", () => {
    expect(stillHeld(employment({ ended_at: null }), ZONE)).toBe(true);
  });

  it("holds a job whose last day is still ahead — a notice period", () => {
    expect(
      stillHeld(employment({ ended_at: "2026-11-17T00:00:00Z" }), ZONE),
    ).toBe(true);
  });

  // The two days either side of the boundary. A comparison written the other way
  // round passes every other case in this file and gets exactly these wrong.
  it("does NOT hold a job whose last day is today", () => {
    expect(
      stillHeld(employment({ ended_at: "2026-08-19T00:00:00Z" }), ZONE),
    ).toBe(false);
  });

  it("holds a job whose last day is tomorrow", () => {
    expect(
      stillHeld(employment({ ended_at: "2026-08-20T00:00:00Z" }), ZONE),
    ).toBe(true);
  });

  it("does NOT hold a job whose last day has passed", () => {
    expect(
      stillHeld(employment({ ended_at: "2026-07-20T00:00:00Z" }), ZONE),
    ).toBe(false);
  });
});

describe("currentEmployer", () => {
  const flaggedAndOngoing = employment({
    relationship_id: "rel-current",
    is_current_primary: true,
  });
  const flaggedButPassed = employment({
    relationship_id: "rel-stale",
    is_current_primary: true,
    ended_at: "2026-07-20T00:00:00Z",
  });
  const unflagged = employment({ relationship_id: "rel-side" });

  it("is the flagged employment that is still theirs", () => {
    expect(
      currentEmployer([unflagged, flaggedAndOngoing], ZONE)?.relationship_id,
    ).toBe("rel-current");
  });

  // The whole reason this module exists: the flag alone is not enough.
  it("is undefined when the flagged employment's last day has passed", () => {
    expect(
      currentEmployer([flaggedButPassed, unflagged], ZONE),
    ).toBeUndefined();
  });

  it("is undefined when nothing is flagged", () => {
    expect(currentEmployer([unflagged], ZONE)).toBeUndefined();
  });

  it("is undefined when there are no employments at all", () => {
    expect(currentEmployer(undefined, ZONE)).toBeUndefined();
    expect(currentEmployer([], ZONE)).toBeUndefined();
  });
});

describe("formerEmployers", () => {
  const current = employment({
    relationship_id: "rel-current",
    is_current_primary: true,
  });
  const past = employment({
    relationship_id: "rel-past",
    ended_at: "2026-01-31T00:00:00Z",
  });

  it("is everything the current employer is not, so the two partition the rows", () => {
    const rows = [current, past];
    const former = formerEmployers(rows, ZONE);
    expect(former.map((e) => e.relationship_id)).toEqual(["rel-past"]);
    expect(former.length + 1).toBe(rows.length);
  });

  // A stale flag must not hide a row from BOTH lists — that is how somebody
  // disappears off their own record entirely.
  it("includes a flagged employment whose last day has passed", () => {
    const stale = employment({
      relationship_id: "rel-stale",
      is_current_primary: true,
      ended_at: "2026-07-20T00:00:00Z",
    });
    expect(
      formerEmployers([stale], ZONE).map((e) => e.relationship_id),
    ).toEqual(["rel-stale"]);
  });

  it("is empty when there are no employments", () => {
    expect(formerEmployers(undefined, ZONE)).toEqual([]);
  });
});

it("keeps undated history out of current employment and respects month precision", () => {
  expect(stillHeld(employment({ employment_status: "unknown" }), ZONE)).toBe(
    false,
  );
  expect(stillHeld(employment({ employment_status: "former" }), ZONE)).toBe(
    false,
  );
  expect(
    stillHeld(
      employment({
        employment_status: "current",
        ended_at: "2026-08-01",
        ended_precision: "month",
      }),
      ZONE,
    ),
  ).toBe(true);
  expect(
    stillHeld(
      employment({
        employment_status: "current",
        ended_at: "2026-07-01",
        ended_precision: "month",
      }),
      ZONE,
    ),
  ).toBe(false);
});

it.each(currencyCases)("matches the shared server corpus: $name", (sample) => {
  vi.setSystemTime(new Date(`${sample.today}T12:00:00Z`));
  const status =
    sample.status === "current" ||
    sample.status === "former" ||
    sample.status === "unknown"
      ? sample.status
      : undefined;
  const precision =
    sample.precision === "month" || sample.precision === "day"
      ? sample.precision
      : undefined;
  expect(
    stillHeld(
      employment({
        ended_at: sample.end,
        employment_status: status,
        ended_precision: precision,
      }),
      ZONE,
    ),
  ).toBe(sample.current);
});
