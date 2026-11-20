// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type InfiniteData, QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import {
  forgetHiddenHomeRows,
  hideHomeRows,
  withoutActedOnRows,
} from "./homerows";
import { homeDayOptions } from "./worklistday";

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

afterEach(forgetHiddenHomeRows);

const row = (id: string, source = "task") => ({ id, source }) as WorklistItem;

function day(queue: string[], focus: string[] = []): Worklist {
  return {
    queue: queue.map((id) => row(id)),
    focus: { items: focus.map((id) => row(id)) },
  } as Worklist;
}

function cached(client: QueryClient, pages: Worklist[]) {
  const key = homeDayOptions().queryKey;
  client.setQueryData<InfiniteData<Worklist>>(key, {
    pages,
    pageParams: pages.map(() => undefined),
  });
  return key;
}

const idsOf = (data: InfiniteData<Worklist> | undefined) =>
  data?.pages.flatMap((p) => [
    ...p.queue.map((i) => i.id),
    ...(p.focus?.items.map((i) => i.id) ?? []),
  ]);

describe("a row the reader acted on", () => {
  it("leaves the cached Home day at once, queue and Focus, and the day shows Updating", async () => {
    const client = new QueryClient();
    const key = cached(client, [day(["a", "b"], ["a"]), day(["c"])]);
    await hideHomeRows(client, [{ id: "a" }]);
    const after = client.getQueryData<InfiniteData<Worklist>>(key);
    expect(idsOf(after)).toEqual(["b", "c"]);
    expect(after?.pages[0]?.served_stale).toBe(true);
    expect(after?.pages[1]?.served_stale).toBeUndefined();
  });

  it("matches the lane only when the action names one", async () => {
    const client = new QueryClient();
    const key = cached(client, [
      {
        queue: [row("x", "task"), row("x", "meeting")],
      } as Worklist,
    ]);
    await hideHomeRows(client, [{ id: "x", source: "meeting" }]);
    const after = client.getQueryData<InfiniteData<Worklist>>(key);
    expect(after?.pages[0]?.queue.map((i) => i.source)).toEqual(["task"]);
  });

  it("comes back, unmarked, when the write fails", async () => {
    const client = new QueryClient();
    const key = cached(client, [day(["a", "b"])]);
    const hide = await hideHomeRows(client, [{ id: "a" }]);
    hide.restore();
    const after = client.getQueryData<InfiniteData<Worklist>>(key);
    expect(idsOf(after)).toEqual(["a", "b"]);
    expect(after?.pages[0]?.served_stale).toBeUndefined();
    expect(
      idsOf({ pages: [withoutActedOnRows(day(["a"]), true)] } as never),
    ).toEqual(["a"]);
  });

  it("stays out of a kept day read afterwards, and returns with the first fresh one", async () => {
    const client = new QueryClient();
    cached(client, [day(["a", "b"])]);
    const hide = await hideHomeRows(client, [{ id: "a" }]);
    hide.settle();
    const stale = { ...day(["a", "b"]), served_stale: true } as Worklist;
    expect(withoutActedOnRows(stale, true).queue.map((i) => i.id)).toEqual([
      "b",
    ]);
    const fresh = day(["a", "b"]);
    expect(withoutActedOnRows(fresh, true).queue.map((i) => i.id)).toEqual([
      "a",
      "b",
    ]);
  });

  it("stays out of every read while the write is still on its way", async () => {
    const client = new QueryClient();
    cached(client, [day(["a"])]);
    await hideHomeRows(client, [{ id: "a" }]);
    expect(withoutActedOnRows(day(["a", "b"]), true).queue).toHaveLength(1);
  });

  it("is forgotten after two minutes", async () => {
    const client = new QueryClient();
    cached(client, [day(["a"])]);
    await hideHomeRows(client, [{ id: "a" }], 0);
    expect(withoutActedOnRows(day(["a"]), true, 3 * 60_000).queue).toHaveLength(
      1,
    );
  });
});
