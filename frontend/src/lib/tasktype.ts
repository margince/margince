// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { valueFor } from "./labelfor";

export type TaskType = components["schemas"]["TaskType"];
type Activity = components["schemas"]["Activity"];

/** Every type, in the order a picker lists them. */
export const TASK_TYPES: readonly TaskType[] = [
  "reply",
  "write",
  "call",
  "meet",
  "review",
  "decide",
  "do",
];

const TASK_TYPE_LABELS: Readonly<Record<TaskType, MessageKey>> = {
  reply: "task.type.reply",
  write: "task.type.write",
  call: "task.type.call",
  meet: "task.type.meet",
  review: "task.type.review",
  decide: "task.type.decide",
  do: "task.type.do",
};

/**
 * The type a task's one step follows. A server too old to send one still
 * names a mail target where a mail is the work, so that reads as `write`.
 * Anything else, or a type this build does not know, reads as `do`, since
 * Done is the step every task has.
 */
export function taskTypeOf(
  task: Pick<Activity, "task_type" | "email_target">,
): TaskType {
  const sent = task.task_type;
  if (sent && isTaskType(sent)) return sent;
  return !sent && task.email_target ? "write" : "do";
}

export function isTaskType(value: string): value is TaskType {
  return Object.hasOwn(TASK_TYPE_LABELS, value);
}

export function taskTypeLabel(type: TaskType): MessageKey {
  return valueFor(TASK_TYPE_LABELS, type, TASK_TYPE_LABELS.do);
}
