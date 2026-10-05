import { describe, expect, it } from "vitest";
import { formatRelativeTime } from "./relativetime";

describe("formatRelativeTime", () => {
  const now = new Date("2026-10-05T12:00:00Z");

  it("reads the past and the future in the largest whole unit", () => {
    expect(formatRelativeTime("2026-10-02T12:00:00Z", "en", now)).toBe(
      "3 days ago",
    );
    expect(formatRelativeTime("2026-10-05T09:00:00Z", "en", now)).toBe(
      "3 hours ago",
    );
    expect(formatRelativeTime("2026-10-05T12:15:00Z", "en", now)).toBe(
      "in 15 minutes",
    );
  });

  it("follows the reader's language", () => {
    expect(formatRelativeTime("2026-10-05T09:00:00Z", "de", now)).toBe(
      "vor 3 Stunden",
    );
  });
});
