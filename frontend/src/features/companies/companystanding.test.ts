import { describe, expect, it } from "vitest";
import {
  companyDimRows,
  companyVerdict,
  type RatedDimension,
} from "./companystanding";

const dims = (
  relationship?: RatedDimension["rating"],
  commercial?: RatedDimension["rating"],
  payment?: RatedDimension["rating"],
): RatedDimension[] => [
  { key: "relationship", rating: relationship, reason: "Last met 12 days ago" },
  { key: "commercial", rating: commercial, reason: "1 of 2 deals stalled" },
  { key: "payment", rating: payment, reason: "4 of 4 paid on time" },
];

const owed = { rule: "overdue_promise" as const, why_now: "Due 25 Jul" };
const quiet = { rule: "nothing_needed" as const, why_now: "All caught up" };

describe("companyVerdict", () => {
  it.each([
    {
      name: "nothing rated reads not assessed, in no state colour",
      input: { dimensions: dims(), withheld: false },
      want: { word: "standing.notAssessed", tone: "neutral" },
    },
    {
      name: "a withheld health section says so, not that nobody judged it",
      input: { dimensions: dims(), withheld: true },
      want: { word: "record.notShown", tone: "neutral" },
    },
    {
      name: "the worst rating wins over a strong one",
      input: { dimensions: dims("strong", "at_risk"), withheld: false },
      want: {
        word: "co.health.rating.atRisk",
        tone: "danger",
        sentence: "1 of 2 deals stalled",
      },
    },
    {
      name: "one rated dimension is the call, with its own reason",
      input: { dimensions: dims("good"), withheld: false },
      want: {
        word: "co.health.rating.good",
        tone: "warning",
        sentence: "Last met 12 days ago",
      },
    },
    {
      name: "what is owed stays in the Next bar, and the reason is the sentence",
      input: {
        dimensions: dims("strong", "strong", "strong"),
        withheld: false,
        moment: owed,
      },
      want: {
        word: "co.health.rating.strong",
        tone: "success",
        sentence: "Last met 12 days ago",
      },
    },
    {
      name: "with nothing rated, what is owed is the sentence",
      input: { dimensions: dims(), withheld: false, moment: owed },
      want: {
        word: "standing.notAssessed",
        tone: "neutral",
        sentence: "Due 25 Jul",
      },
    },
    {
      name: "the quiet rung leaves the reason as the sentence",
      input: { dimensions: dims("strong"), withheld: false, moment: quiet },
      want: {
        word: "co.health.rating.strong",
        tone: "success",
        sentence: "Last met 12 days ago",
      },
    },
  ])("$name", ({ input, want }) => {
    expect(companyVerdict(input)).toEqual(want);
  });
});

describe("companyDimRows", () => {
  it("keeps the ring's order and greys what nobody rated", () => {
    expect(companyDimRows(dims("good", undefined, "strong"), false)).toEqual([
      {
        key: "relationship",
        label: "co.health.dim.relationship",
        value: "co.health.rating.good",
        sub: "Last met 12 days ago",
        tone: "warning",
      },
      {
        key: "commercial",
        label: "co.health.dim.commercial",
        value: "standing.notAssessed",
        sub: undefined,
        tone: "neutral",
      },
      {
        key: "payment",
        label: "co.health.dim.payment",
        value: "co.health.rating.strong",
        sub: "4 of 4 paid on time",
        tone: "success",
      },
    ]);
  });
});
