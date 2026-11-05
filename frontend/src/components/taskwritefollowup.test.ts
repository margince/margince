import { QueryClient } from "@tanstack/react-query";
import { afterEach, expect, test, vi } from "vitest";
import type { components } from "../api/schema";
import { readingsDay, taskRow } from "../testing/brief.fixtures";
import { refreshMovedTask } from "./taskwritefollowup";

afterEach(() => vi.useRealTimers());

const movedTask: components["schemas"]["Activity"] = {
  id: "task-1",
  kind: "task",
  subject: "Call the buyer",
  occurred_at: "2026-11-01T10:00:00Z",
  occurred_precision: "instant",
  due_at: "2026-11-06T14:00:00Z",
  due_has_time: true,
  version: 2,
  offers_email: false,
  is_done: false,
  source: "manual",
  captured_by: "human:user-1",
  created_at: "2026-11-01T10:00:00Z",
  updated_at: "2026-11-01T10:00:00Z",
};

test("moving a task updates its queue and focus without touching reconnections or another source", async () => {
  vi.setSystemTime(new Date("2026-11-05T10:00:00Z"));
  const client = new QueryClient();
  const key = ["worklist", "mine", "all", "", "today"];
  const row = { ...taskRow("task-1", "Call the buyer"), overdue: true };
  const other = { ...row, source: "meeting" } as const;
  const day = readingsDay({}, [row, other]);
  client.setQueryData(key, { pages: [day], pageParams: [undefined] });
  const relationships = { items: [{ id: "contact-1" }] };
  const relationshipKey = ["worklist", "relationships", "mine", ""];
  client.setQueryData(relationshipKey, relationships);

  await refreshMovedTask(client, [["worklist"]], movedTask);

  expect(client.getQueryData(["activity", "task-1"])).toEqual(movedTask);
  expect(client.getQueryData(relationshipKey)).toBe(relationships);
  const cached = client.getQueryData<{
    pages: components["schemas"]["Worklist"][];
  }>(key);
  expect(cached?.pages[0]?.queue[0]).toMatchObject({
    due_at: movedTask.due_at,
    version: 2,
    overdue: false,
  });
  expect(cached?.pages[0]?.focus?.items[0]).toEqual(cached?.pages[0]?.queue[0]);
  expect(cached?.pages[0]?.queue[1]).toEqual(other);
  client.clear();
});
