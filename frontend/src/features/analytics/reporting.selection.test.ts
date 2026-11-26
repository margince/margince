// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, expectTypeOf, it } from "vitest";
import type { components, paths } from "../../api/schema";
import type { TEAM_PERIODS } from "./analytics.overview.context";
import {
  customIntervalRefusal,
  PRESET_PERIODS,
  type REPORTING_PERIODS,
  readOverviewFilters,
  validDateRange,
} from "./reporting.selection";

type Schemas = components["schemas"];
type TeamReviewQuery = NonNullable<
  paths["/analytics/sdr-team"]["get"]["parameters"]["query"]
>;

// The type check (tsc -b) fails these when the contract and a list part ways.
describe("each period list is the contract's own enum", () => {
  it("lists every reporting period", () => {
    expectTypeOf<(typeof REPORTING_PERIODS)[number]>().toEqualTypeOf<
      Schemas["ReportingSelection"]["period"]
    >();
  });

  it("lists every period words can name", () => {
    expectTypeOf<(typeof PRESET_PERIODS)[number]>().toEqualTypeOf<
      NonNullable<Schemas["ReportSetupSelection"]["period"]>
    >();
    expect(PRESET_PERIODS).not.toContain("custom");
  });

  it("lists every period the team review reads", () => {
    expectTypeOf<(typeof TEAM_PERIODS)[number]>().toEqualTypeOf<
      NonNullable<TeamReviewQuery["period"]>
    >();
  });
});

describe("a completed fiscal period from the address", () => {
  it.each([
    ["last_quarter", "fiscal_quarter"],
    ["last_year", "month"],
    ["this_quarter", "fiscal_quarter"],
    ["last_month", "month"],
  ] as const)("reads %s against a %s target", (period, basis) => {
    const filters = readOverviewFilters(new Map([["period", period]]), []);
    expect(filters.period).toBe(period);
    expect(filters.targetBasis).toBe(basis);
  });
});

// The server's own refusals, in its order, so a refused interval is never asked.
describe("a custom interval the server would refuse", () => {
  const today = "2026-11-20";
  it.each([
    ["2026-10-20", "2026-10-10", "end_before_start"],
    ["2025-11-01", "2026-11-01", "longer_than_a_year"],
    ["2024-02-29", "2025-03-01", "longer_than_a_year"],
    ["2026-11-21", "2026-11-30", "starts_after_cutoff"],
  ])("from %s through %s is refused as %s", (start, end, reason) => {
    expect(customIntervalRefusal("custom", start, end, today)).toBe(reason);
    expect(validDateRange("custom", start, end, today)).toBe(false);
  });

  it.each([
    ["2026-10-10", "2026-10-10"],
    ["2025-11-01", "2026-10-31"],
    ["2024-02-29", "2025-02-28"],
    ["2026-11-20", "2026-12-31"],
  ])("from %s through %s is asked", (start, end) => {
    expect(customIntervalRefusal("custom", start, end, today)).toBeNull();
    expect(validDateRange("custom", start, end, today)).toBe(true);
  });

  it("judges no day the reader has not chosen, and no named period", () => {
    expect(customIntervalRefusal("custom", "", "2026-10-10", today)).toBeNull();
    expect(validDateRange("custom", "", "2026-10-10", today)).toBe(false);
    expect(
      validDateRange("this_month", "2026-10-20", "2026-10-10", today),
    ).toBe(true);
  });
});
