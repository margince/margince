// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import type { components } from "../../api/schema";
import { soonestFirst } from "./partners.nextstep";

type Activity = components["schemas"]["Activity"];

const task = (id: string, over: Partial<Activity>): Activity =>
  ({ id, kind: "task", subject: id, ...over }) as Activity;

it("orders a task with only a date by its picked day, not by its instant", () => {
  // Picked for 16 Nov in Los Angeles: 07:59 UTC on the 17th, later than the
  // timed task below, yet an earlier day on the company's calendar.
  const dated = task("dated", {
    due_at: "2026-11-17T07:59:59Z",
    due_has_time: false,
    due_intent: {
      wall_clock: "2026-11-16T23:59:59",
      timezone: "America/Los_Angeles",
    },
  });
  const timed = task("timed", {
    due_at: "2026-11-17T06:00:00Z",
    due_has_time: true,
  });
  const undated = task("undated", {});
  expect(
    soonestFirst([undated, timed, dated], "Europe/Berlin").map((t) => t.id),
  ).toEqual(["dated", "timed", "undated"]);
});
