// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { isTaskType, TASK_TYPES, taskTypeOf } from "./tasktype";

it("reads the type the server sent", () => {
  for (const type of TASK_TYPES) {
    expect(taskTypeOf({ task_type: type })).toBe(type);
  }
});

it("reads a missing type as a mail to write where the server names a target", () => {
  const target = { entity_type: "contact", entity_id: "c-1" } as const;
  expect(taskTypeOf({ email_target: target })).toBe("write");
  expect(taskTypeOf({ task_type: "fax", email_target: target })).toBe("do");
});

it("reads a missing or unknown type as a task to do", () => {
  expect(taskTypeOf({})).toBe("do");
  expect(taskTypeOf({ task_type: null })).toBe("do");
  expect(taskTypeOf({ task_type: "fax" })).toBe("do");
  expect(isTaskType("constructor")).toBe(false);
});
