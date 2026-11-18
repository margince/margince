import { describe, expect, it } from "vitest";
import { translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  CLOSE_DAY_DIAL,
  closeDayRange,
  VALUE_DIAL,
} from "../../lib/dealranges";
import {
  type DealFilters,
  dealsByStageReportFilters,
  dealsQueryParams,
  totalsWithheldBecause,
} from "../../lib/deals.queries";
import { dealSurfaceChips } from "./deal360/dealfilterchips";
import { dealFilterChips } from "./deals.listtools";
import { dealRangeChips } from "./deals.rangefilters";

// Every filter the Deals screen offers narrows what it draws. The census reads
// the chips themselves, so a chip whose value the request leaves out fails here.

const t = (key: MessageKey, params?: Record<string, string>) =>
  translate("en", key, params);

// Every conditional chip is offered: each has the options it waits for.
const OFFERED = [
  ...dealFilterChips([], t, undefined),
  ...dealRangeChips({
    words: { t, locale: "en", zone: "UTC" },
    filters: {},
    baseCurrency: "EUR",
    withheld: new Set(),
    leading: ["EUR"],
  }),
  ...dealSurfaceChips({
    partnerOptions: [{ value: "partner-1", label: "Northwind" }],
    acquisitionSources: [
      {
        id: "src-1",
        key: "outbound",
        label: "Outbound",
        sort_order: 1,
        active: true,
        system: true,
        version: 1,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ],
    linkTypes: [{ name: "renewal_of", label: "Renewal of", from: [], to: [] }],
    retiredSuffix: "",
  }),
].map((chip) => ({
  key: chip.key,
  values: chip.options.map((option) => option.value),
}));

// A value for each chip that is typed or searched rather than picked.
const TYPED: Readonly<Record<string, string>> = {
  stage_id: "stage-1",
  company_id: "company-1",
  [CLOSE_DAY_DIAL]: "2026-01-01..2026-03-31",
  [VALUE_DIAL]: "1000..",
};

const LONG_PICKER = 12;

function filtersOf(filters: Record<string, string>): DealFilters {
  return {
    pipelineId: "pipeline-1",
    q: "",
    sort: "",
    includeArchived: false,
    filters,
    closeDay: closeDayRange(filters[CLOSE_DAY_DIAL], "2026-11-19", 1),
  };
}

// One case per option. A long picker, a currency or a country, sends every
// option the same way, so its first stands for the rest.
const CASES = OFFERED.flatMap(({ key, values }) => {
  const sent = values.length > LONG_PICKER ? values.slice(0, 1) : values;
  return (sent.length > 0 ? sent : [TYPED[key] ?? ""]).map(
    (value) => [key, value] as const,
  );
});

describe("every deals filter", () => {
  it("is in the census", () => {
    expect(CASES.map(([key]) => key)).toEqual(
      expect.arrayContaining([
        "commercial_motion",
        "priority",
        "acquisition_source",
        "link_type",
      ]),
    );
  });

  it.each(CASES)("%s=%s changes the /deals request", (key, value) => {
    expect(value, `${key} has no option and no typed value`).not.toBe("");
    const narrowed = filtersOf({ [key]: value });
    expect(dealsQueryParams(narrowed)).not.toEqual(
      dealsQueryParams(filtersOf({})),
    );
  });

  // A stage total the report counts over a wider set than the cards would
  // print a number the column does not hold.
  it.each(CASES)(
    "%s=%s narrows the stage totals or withholds them",
    (key, value) => {
      const narrowed = filtersOf({ [key]: value });
      if (totalsWithheldBecause(narrowed)) return;
      expect(dealsByStageReportFilters(narrowed)).not.toEqual(
        dealsByStageReportFilters(filtersOf({})),
      );
    },
  );
});
