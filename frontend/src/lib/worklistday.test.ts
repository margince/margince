// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import {
  readWorklistPage,
  untilNextDue,
  WORKLIST_PAGE_ROWS,
  worklistDayOptions,
} from "./worklistday";

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

const now = Date.parse("2026-10-31T09:00:00Z");

function page(queue: string[], focus: string[] = []): Worklist {
  const row = (due_at: string) => ({ due_at }) as WorklistItem;
  return {
    queue: queue.map(row),
    focus: { items: focus.map(row) },
  } as Worklist;
}

describe("the Home day's refresh", () => {
  it("comes when the soonest loaded row comes due, across pages and Focus", () => {
    const pages = [
      page(["2026-10-31T08:00:00Z", "2026-10-31T11:00:00Z"]),
      page(["2026-10-31T10:00:00Z"], ["2026-10-31T09:30:00Z"]),
    ];
    expect(untilNextDue(pages, now)).toBe(30 * 60_000 + 1_000);
  });

  it("does not come at all when no loaded row is still to come due", () => {
    expect(untilNextDue([page(["2026-10-31T08:00:00Z"])], now)).toBe(false);
    expect(untilNextDue([], now)).toBe(false);
  });

  it("replaces the minute poll on the reader's own day only", () => {
    const home = worklistDayOptions("mine", "all");
    const team = worklistDayOptions("team", "all");
    expect(typeof home.refetchInterval).toBe("function");
    expect(team.refetchInterval).toBeUndefined();
    expect(team.refetchOnWindowFocus).toBe(true);
  });
});

describe("a day page", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("asks for the largest page, so scrolling rarely reads the day again", async () => {
    const asked: URL[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        asked.push(
          new URL(input instanceof Request ? input.url : String(input)),
        );
        return new Response(JSON.stringify(page([])), {
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    await readWorklistPage("mine", "all", undefined, undefined);
    await readWorklistPage("mine", "all", undefined, "next");
    expect(asked.map((url) => url.searchParams.get("limit"))).toEqual([
      String(WORKLIST_PAGE_ROWS),
      String(WORKLIST_PAGE_ROWS),
    ]);
    expect(WORKLIST_PAGE_ROWS).toBe(100);
  });
});
