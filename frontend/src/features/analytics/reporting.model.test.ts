import { expect, it } from "vitest";
import { reportingAmount } from "./reporting.model";

it("formats API currency units using the currency minor-unit scale", () => {
  expect(reportingAmount(21600000, "EUR", "EUR", "en")).toBe("€216,000.00");
  expect(reportingAmount(216000, "JPY", "JPY", "en")).toBe("JP¥216,000");
  expect(reportingAmount(12, "count", "EUR", "en")).toBe("12");
  expect(reportingAmount(12, "count", "count", "en")).toBe("12");
  expect(reportingAmount(null, "EUR", "EUR", "en")).toBe("—");
});
