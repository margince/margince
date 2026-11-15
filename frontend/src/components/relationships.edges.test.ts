import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { dateRange } from "./relationships.edges";

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
    expect(dateRange(job({ employment_status: "current" }), t)).toBe(
      "rel.current",
    );
    expect(dateRange(job({ employment_status: "former" }), t)).toBe(
      "rel.former",
    );
    expect(dateRange(job({ employment_status: "unknown" }), t)).toBe("");
  });

  it("keeps a stated end date and a non-employment edge as they were", () => {
    expect(
      dateRange(job({ started_at: "2024-03", ended_at: "2025-01" }), t),
    ).toBe("2024-03 – 2025-01");
    expect(dateRange(job({ kind: "works_with" }), t)).toBe("rel.current");
  });
});
