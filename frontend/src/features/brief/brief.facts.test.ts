import { expect, it } from "vitest";
import { briefDay } from "./brief.facts";
import { readingsDay, taskRow } from "./brief.fixtures";

it("retains a source failure discovered while loading another page", () => {
  const first = readingsDay({}, [taskRow("first", "Call Weber")]);
  const last = readingsDay({ more_available: true }, [
    taskRow("second", "Review renewal"),
  ]);
  last.sources_unavailable = [
    { source: "deal_at_risk", reason: "failed", category: "deals_at_risk" },
  ];
  const combined = briefDay([first, last]);
  expect(combined?.queue.map((row) => row.id)).toEqual(["first", "second"]);
  expect(combined?.sources_unavailable).toEqual(last.sources_unavailable);
  expect(combined?.readings.more_available).toBe(true);
});
