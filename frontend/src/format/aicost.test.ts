import { describe, expect, it } from "vitest";
import { formatAiRunCost } from "./aicost";

describe("formatAiRunCost", () => {
  it("keeps a fraction of a cent visible", () => {
    expect(formatAiRunCost(4300, "en")).toBe("US$0.0043");
    expect(formatAiRunCost(123, "en")).toBe("US$0.000123");
  });

  it("shows a cent or more with two decimals, and zero as zero", () => {
    expect(formatAiRunCost(25_000, "en")).toBe("US$0.025");
    expect(formatAiRunCost(1_500_000, "en")).toBe("US$1.50");
    expect(formatAiRunCost(0, "en")).toBe("US$0.00");
  });

  it("uses the reader's conventions", () => {
    expect(formatAiRunCost(4300, "de")).toBe("0,0043\u00a0$");
  });
});
