/** @vitest-environment happy-dom */
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { jsonResponse, stubApi } from "../testing/brief.testkit";
import {
  beginUndoManifest,
  keepUndoManifest,
  readUndoRuns,
  resumeUndoRuns,
} from "./marginchanges.runs";
import { isRecord } from "./problembody";
import { forgetSeat, readStored, STORAGE_KEYS } from "./storage";

type Run = components["schemas"]["MarginceUndoRun"];

afterEach(() => {
  forgetSeat();
  vi.unstubAllGlobals();
});

it("recovers the original results of every batch after the last response is lost", async () => {
  const saved = new Map<string, Run>();
  const items = Array.from({ length: 201 }, (_, index) => ({
    deal_id: `deal-${index}`,
    version: 7,
    audit_log_ids: [`change-${index}`],
  }));
  let posts = 0;
  stubApi({
    "POST /deal-corrections/undo/results": () =>
      jsonResponse({ runs: [...saved.values()] }),
    "POST /deal-corrections/undo": (body) => {
      if (!isRecord(body) || typeof body.run_id !== "string")
        throw new Error("missing durable run id");
      const chosen = items.slice(posts * 200, (posts + 1) * 200);
      expect(body.items).toEqual(chosen);
      const run: Run = {
        run_id: body.run_id,
        created_at: "2026-09-13T02:00:00Z",
        complete: true,
        items: chosen,
        results: chosen.map((item) => ({
          deal_id: item.deal_id,
          outcome: "undone",
          undone: ["close_lost"],
          kept: [],
        })),
      };
      saved.set(run.run_id, run);
      posts++;
      return posts === 2
        ? jsonResponse({ title: "Unavailable" }, 503)
        : jsonResponse(run);
    },
  });
  const manifest = keepUndoManifest(items);
  const first = await resumeUndoRuns(manifest);
  expect(first.results).toHaveLength(201);
  expect(first.unanswered).toHaveLength(0);
  expect(first.error).toBeUndefined();
  const recovered = await readUndoRuns(manifest);
  expect(recovered.flatMap((run) => run.results)).toHaveLength(201);
  const retried = await resumeUndoRuns(manifest);
  expect(retried.results).toHaveLength(201);
  expect(retried.unanswered).toEqual([]);
  expect(posts).toBe(2);
  expect(readStored(STORAGE_KEYS.lastBulkUndo)).toBe(JSON.stringify(manifest));
  forgetSeat();
  expect(readStored(STORAGE_KEYS.lastBulkUndo)).toBeNull();
});

it("resumes an unfinished batch with exactly its original request and run id", async () => {
  const item = { deal_id: "deal-1", version: 7, audit_log_ids: ["change-1"] };
  const manifest = keepUndoManifest([item]);
  const calls = stubApi({
    "POST /deal-corrections/undo/results": () =>
      jsonResponse({
        runs: [
          {
            run_id: manifest.batches[0].id,
            created_at: "2026-09-13T02:00:00Z",
            items: [item],
            results: [],
            complete: false,
          },
        ],
      }),
    "POST /deal-corrections/undo": () =>
      jsonResponse({
        results: [
          {
            deal_id: item.deal_id,
            outcome: "undone",
            undone: ["close_lost"],
            kept: [],
          },
        ],
        complete: true,
      }),
  });
  await resumeUndoRuns(manifest);
  expect(
    calls.find((call) => call.path === "/deal-corrections/undo")?.body,
  ).toEqual({ run_id: manifest.batches[0].id, items: [item] });
});

it("keeps an unfinished earlier attempt reachable when another bulk Undo starts", async () => {
  const first = keepUndoManifest([
    { deal_id: "old", version: 7, audit_log_ids: ["old-change"] },
  ]);
  stubApi({
    "POST /deal-corrections/undo/results": () => jsonResponse({ runs: [] }),
  });
  const latest = await beginUndoManifest([
    { deal_id: "new", version: 9, audit_log_ids: ["new-change"] },
  ]);
  expect(latest.batches).toHaveLength(1);
  expect(latest.batches[0]?.items[0]?.deal_id).toBe("new");
  expect(JSON.parse(readStored(STORAGE_KEYS.lastBulkUndo) ?? "null")).toEqual({
    batches: [...first.batches, ...latest.batches],
  });
});
