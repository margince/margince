import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { toBoardDeal } from "./boarddeal";
import type { CompanyNaming } from "./dealcompanymarks";

type Deal = components["schemas"]["Deal"];

const noCompany: CompanyNaming = { marks: new Map(), unreadable: new Set() };

function deal(overrides: Partial<Deal>): Deal {
  return {
    id: "d1",
    name: "Fleet retrofit",
    pipeline_id: "pl",
    stage_id: "s1",
    status: "open",
    source: "manual",
    captured_by: "human:u1",
    version: 4,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  } as Deal;
}

describe("toBoardDeal", () => {
  // The wire's last_email reaches the card as a span and a direction, or the
  // board would date every deal's mail as "just now" — the same copy-across
  // rule the close date holds.
  it("carries when mail last moved and which way", () => {
    const tenDaysAgo = new Date(Date.now() - 10 * 86_400_000).toISOString();
    const card = toBoardDeal(
      deal({ last_email: { occurred_at: tenDaysAgo, direction: "outbound" } }),
      noCompany,
    );
    expect(card.lastEmail?.direction).toBe("outbound");
    expect(Math.round((card.lastEmail?.agoMs ?? 0) / 86_400_000)).toBe(10);
  });

  // A mail stamped a few seconds into the reader's future is clock skew, not
  // a negative age; a deal nobody mailed about states no mail line at all.
  it("floors a future instant at zero and leaves an unmailed deal blank", () => {
    const ahead = new Date(Date.now() + 5_000).toISOString();
    expect(
      toBoardDeal(
        deal({ last_email: { occurred_at: ahead, direction: null } }),
        noCompany,
      ).lastEmail,
    ).toEqual({ agoMs: 0, direction: null });
    expect(toBoardDeal(deal({}), noCompany).lastEmail).toBeNull();
  });
});
