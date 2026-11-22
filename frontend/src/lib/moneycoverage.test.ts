import { expect, it } from "vitest";
import { scopeDealDials } from "./dealsaddress";
import { addCoverage, coverageOf, hasGap } from "./moneycoverage";

const stated = (
  noValue: number,
  noRate: number,
  currencies: string[] = [],
) => ({
  coverage: {
    aggregates: ["raw"],
    partial: noValue + noRate > 0,
    no_value_deals: noValue,
    no_rate_deals: noRate,
    missing_currencies: currencies,
  },
});

it("reads the two counts apart and says nothing when the row states none", () => {
  expect(coverageOf(stated(2, 1, ["JPY"]))).toEqual({
    partial: true,
    noValue: 2,
    noRate: 1,
    rateCurrencies: ["JPY"],
  });
  expect(coverageOf({ deals: 3 })).toBeNull();
  expect(hasGap(coverageOf(stated(0, 0)))).toBe(false);
});

it("adds the gaps of several groups, and none when one group did not say", () => {
  const jpy = coverageOf(stated(1, 1, ["JPY"]));
  const sek = coverageOf(stated(0, 2, ["JPY", "SEK"]));
  expect(addCoverage([jpy, sek])).toEqual({
    partial: true,
    noValue: 1,
    noRate: 3,
    rateCurrencies: ["JPY", "SEK"],
  });
  expect(addCoverage([jpy, null])).toBeNull();
});

it("names the deals-list dials a scope counts, and none where the list cannot say it", () => {
  const scope = (kind: string, id?: string) => ({ kind, id });
  expect(scopeDealDials(scope("workspace"))).toEqual({});
  expect(scopeDealDials(scope("owner", "u1"))).toEqual({ owner_id: "u1" });
  expect(scopeDealDials(scope("team", "t1"))).toEqual({ owner_team_id: "t1" });
  expect(scopeDealDials(scope("managed_teams"))).toBeNull();
});
