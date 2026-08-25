import { describe, expect, it } from "vitest";

import { idleSince } from "./idlebase";

describe("idleSince", () => {
  it("reads the newest activity when the record has one", () => {
    expect(
      idleSince({
        last_activity_at: "2026-03-02T14:30:00Z",
        created_at: "2026-01-05T09:00:00Z",
      }),
    ).toBe("2026-03-02T14:30:00Z");
  });

  // The case the fallback exists for. A record nobody has ever touched has
  // been silent since the day somebody wrote it down, and treating that as
  // unknown hides the oldest untouched records from the board that ranks them.
  it("falls back to the creation of a record nothing has touched", () => {
    expect(
      idleSince({ last_activity_at: null, created_at: "2026-01-05T09:00:00Z" }),
    ).toBe("2026-01-05T09:00:00Z");
    expect(idleSince({ created_at: "2026-01-05T09:00:00Z" })).toBe(
      "2026-01-05T09:00:00Z",
    );
  });
});
