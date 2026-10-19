import type { components } from "../../api/schema";

type WorklistItem = components["schemas"]["WorklistItem"];

export function row(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "row-1",
    source: "task",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    because: [],
    actions: [],
    // The version the row's own verbs write with. Present on every fixture row
    // because the server sends it for every row whose verbs write: a fixture
    // without one describes a payload the read cannot produce, and the write
    // under test would refuse rather than run.
    version: 3,
    ...over,
  };
}
