import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { formatDate } from "../format/format";
import type { MessageKey } from "../i18n/en";
import { dateRange, mayRemoveEdge } from "./relationships.edges";

type Relationship = components["schemas"]["Relationship"];

const t = (key: MessageKey) => key;
const job = (over: Partial<Relationship>): Relationship => ({
  id: "r-1",
  kind: "employment",
  is_current_primary: false,
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  ...over,
});

describe("dateRange", () => {
  it("calls an undated job current only while the contact page would", () => {
    expect(
      dateRange(job({ employment_status: "current" }), t, "en", "UTC", "UTC"),
    ).toBe("rel.current");
    expect(
      dateRange(job({ employment_status: "former" }), t, "en", "UTC", "UTC"),
    ).toBe("rel.former");
    expect(
      dateRange(job({ employment_status: "unknown" }), t, "en", "UTC", "UTC"),
    ).toBe("");
  });

  it("keeps a month-precision date a month and a non-employment edge current", () => {
    const months = job({
      started_at: "2024-03-01",
      started_precision: "month",
      ended_at: "2025-01-01",
      ended_precision: "month",
    });
    expect(dateRange(months, t, "en", "UTC", "UTC")).toBe("2024-03 – 2025-01");
    expect(dateRange(job({ kind: "works_with" }), t, "en", "UTC", "UTC")).toBe(
      "rel.current",
    );
  });

  it("writes a day-precision date in the reader's date style", () => {
    const started = job({
      started_at: "2026-12-31",
      started_precision: "day",
      employment_status: "current",
    });
    expect(dateRange(started, t, "en", "UTC", "UTC")).toBe(
      `${formatDate("2026-12-31", "en", "UTC")} – rel.current`,
    );
    expect(dateRange(started, t, "de", "Europe/Berlin", "Europe/Berlin")).toBe(
      "31.12.2026 – rel.current",
    );
  });
});

// The server's removalGrants: a deal stakeholder comes off with the grant that
// put it on, and every other kind keeps needing delete.
describe("mayRemoveEdge", () => {
  it("admits a deal stakeholder to a seat holding create alone", () => {
    expect(
      mayRemoveEdge("deal_stakeholder", { create: true, delete: false }),
    ).toBe(true);
  });

  it("keeps every other kind behind the delete grant", () => {
    for (const kind of ["employment", "project_stakeholder", "partner_of"]) {
      expect(mayRemoveEdge(kind, { create: true, delete: false })).toBe(false);
      expect(mayRemoveEdge(kind, { create: false, delete: true })).toBe(true);
    }
  });

  it("refuses a seat holding neither grant", () => {
    expect(
      mayRemoveEdge("deal_stakeholder", { create: false, delete: false }),
    ).toBe(false);
  });
});
