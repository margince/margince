/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it } from "vitest";
import { taskHref } from "./taskreminders";
import { moveHref } from "./worklistcopy";

afterEach(() => {
  window.location.hash = "";
});

it("keeps the reader's Home dials and adds the task", () => {
  window.location.hash = "#/home?queue=1&filter=tasks&queue_scope=mine";
  const href = taskHref("t1");
  expect(href).toContain("queue=1");
  expect(href).toContain("filter=tasks");
  expect(href).toContain("queue_scope=mine");
  expect(href).toContain("task=t1");
});

it("opens Home with the task alone from another page", () => {
  window.location.hash = "#/deals?sort=value";
  expect(taskHref("t1")).toBe("#/home?task=t1");
});

it("sends a row's Open task to the task, not its deal", () => {
  window.location.hash = "#/home?queue=1";
  const href = moveHref({
    id: "row",
    source: "deal_at_risk",
    subject: { type: "deal", id: "d1" },
    move: { action: "open_task", activity_id: "t9" },
  } as Parameters<typeof moveHref>[0]);
  expect(href).toContain("task=t9");
  expect(href).not.toContain("d1");
});
