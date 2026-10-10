import { describe, expect, it } from "vitest";
import { boardDealCount } from "./dealboardcount";

const stages = [
  { id: "s1", name: "Qualify", position: 1 },
  { id: "s2", name: "Propose", position: 2 },
] as Parameters<typeof boardDealCount>[0];
const loaded = [{ id: "a", stage_id: "s1" }] as Parameters<
  typeof boardDealCount
>[1];

describe("boardDealCount", () => {
  // The header and the columns read one count, the server's.
  it("adds up the columns' own counts, not the loaded cards", () => {
    const totals = new Map([
      ["s1", { count: 77 }],
      ["s2", { count: 29 }],
    ]);
    expect(boardDealCount(stages, loaded, totals)).toBe(106);
  });

  it("falls back to the loaded cards while the totals are loading or refused", () => {
    expect(boardDealCount(stages, loaded)).toBe(1);
    expect(boardDealCount(stages, loaded, null)).toBe(1);
  });
});
