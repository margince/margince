import type { components } from "../../api/schema";

type WorklistItem = components["schemas"]["WorklistItem"];

export function row(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "row-1",
    source: "task",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    because: [],
    actions: [],
    // Writable rows need a version so their actions reach the mutation.
    version: 3,
    ...over,
  };
}
