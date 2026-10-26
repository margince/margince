import { describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
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
  // The wire's last_email reaches the card with the instant its age is
  // measured from, or the board would date every deal's mail as "just now".
  it("carries when mail last moved, how precisely, and which way", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-15T12:00:00Z"));
    const card = toBoardDeal(
      deal({
        last_email: {
          occurred_at: "2026-09-05T12:00:00Z",
          occurred_precision: "day",
          direction: "outbound",
        },
      }),
      noCompany,
    );
    vi.useRealTimers();
    expect(card.lastEmail).toEqual({
      at: "2026-09-05T12:00:00Z",
      precision: "day",
      measuredAt: Date.parse("2026-09-15T12:00:00Z"),
      direction: "outbound",
    });
  });

  it("leaves a deal nobody mailed about without a mail line", () => {
    expect(toBoardDeal(deal({}), noCompany).lastEmail).toBeNull();
  });
});
