// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import type { components } from "../api/schema";
import { isTaskType, TASK_TYPES, taskTypeOf } from "./tasktype";

type TypedTask = Pick<
  components["schemas"]["Activity"],
  "task_type" | "email_target"
>;

// A type a newer server may send, which this build's contract does not name.
function withNewerType(task: TypedTask = {}) {
  return Object.assign(task, { task_type: "fax" });
}

it("reads the type the server sent", () => {
  for (const type of TASK_TYPES) {
    expect(taskTypeOf({ task_type: type })).toBe(type);
  }
});

it("reads a missing type as a mail to write where the server names a target", () => {
  const target = { entity_type: "contact", entity_id: "c-1" } as const;
  expect(taskTypeOf({ email_target: target })).toBe("write");
  expect(taskTypeOf(withNewerType({ email_target: target }))).toBe("do");
});

it("reads a missing or unknown type as a task to do", () => {
  expect(taskTypeOf({})).toBe("do");
  expect(taskTypeOf(withNewerType())).toBe("do");
  expect(isTaskType("constructor")).toBe(false);
});
