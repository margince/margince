// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, expectTypeOf, it } from "vitest";
import type { components, paths } from "../../api/schema";
import type { TEAM_PERIODS } from "./analytics.overview.context";
import {
  PRESET_PERIODS,
  type REPORTING_PERIODS,
  readOverviewFilters,
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
