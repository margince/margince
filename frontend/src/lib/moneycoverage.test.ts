import { expect, it } from "vitest";
import {
  addCoverage,
  coverageOf,
  hasGap,
  withRateCurrencies,
} from "./moneycoverage";

it("reads the two counts apart and says nothing when the row has none", () => {
  expect(coverageOf({ no_value_deals: 2, no_rate_deals: "1" })).toEqual({
    noValue: 2,
    noRate: 1,
    rateCurrencies: [],
  });
  expect(coverageOf({ deals: 3 })).toBeNull();
  expect(hasGap(coverageOf({ no_value_deals: 0, no_rate_deals: 0 }))).toBe(
    false,
  );
});

it("adds the gaps of several groups, and none when one group did not say", () => {
  const jpy = coverageOf({
    no_value_deals: 1,
    no_rate_deals: 1,
    no_rate_currencies: "JPY",
  });
  const sek = coverageOf({
    no_value_deals: 0,
    no_rate_deals: 2,
    no_rate_currencies: "SEK,JPY",
  });
  expect(addCoverage([jpy, sek])).toEqual({
    noValue: 1,
    noRate: 3,
    rateCurrencies: ["JPY", "SEK"],
  });
  expect(addCoverage([jpy, null])).toBeNull();
});

it("names the currencies of each group's no-rate deals, asking only when there are some", async () => {
  const rows = [
    { stage_id: "a", no_rate_deals: 2, no_value_deals: 0 },
    { stage_id: "b", no_rate_deals: 0, no_value_deals: 1 },
  ];
  const asked: string[][] = [];
  const named = await withRateCurrencies(rows, ["stage_id"], async (by) => {
    asked.push(by);
    return [
      { stage_id: "a", currency: "SEK", no_rate_deals: 1 },
      { stage_id: "a", currency: "JPY", no_rate_deals: 1 },
      { stage_id: "b", currency: "EUR", no_rate_deals: 0 },
    ];
  });
  expect(asked).toEqual([["stage_id", "currency"]]);
  expect(named.map((row) => coverageOf(row)?.rateCurrencies)).toEqual([
    ["JPY", "SEK"],
    [],
  ]);

  let calls = 0;
  await withRateCurrencies([rows[1]], ["stage_id"], async () => {
    calls += 1;
    return [];
  });
  expect(calls).toBe(0);
});
