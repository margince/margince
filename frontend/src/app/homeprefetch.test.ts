// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { InfiniteQueryObserver } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { morningBriefOptions } from "../features/brief/brief.queries";
import { worklistDayOptions } from "../lib/worklistday";
import { DEFAULT_VIEW } from "../lib/worklistviews";
import { quietRun } from "../testing/brief.fixtures";
import { prefetchHome } from "./homeprefetch";
import { createQueryClient, liveOnMount } from "./queryclient";

afterEach(() => vi.restoreAllMocks());

const brief = quietRun;

function briefQuery(client: ReturnType<typeof createQueryClient>) {
  const query = client
    .getQueryCache()
    .find({ queryKey: morningBriefOptions.queryKey, exact: true });
  if (!query) throw new Error("the brief read is not in the cache");
  return query;
}

it("lets Home's mount use an answer the prefetch fetched", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
    Response.json(brief),
  );
  const client = createQueryClient();
  prefetchHome(client, new Map());
  await vi.waitFor(() => expect(briefQuery(client).state.data).toEqual(brief));
  await vi.waitFor(() => expect(liveOnMount(briefQuery(client))).toBe(true));
});

it("keeps the mount refetch for an answer already cached before the prefetch", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
    Response.json(brief),
  );
  const client = createQueryClient();
  client.setQueryData(morningBriefOptions.queryKey, brief);
  prefetchHome(client, new Map());
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(liveOnMount(briefQuery(client))).toBe("always");
});

it("prefetches nothing for the weekly view", () => {
  const fetched = vi.spyOn(globalThis, "fetch");
  prefetchHome(createQueryClient(), new Map([["view", "weekly"]]));
  expect(fetched).not.toHaveBeenCalled();
});

// The Worklist read hangs until its signal aborts, and the test hands back that
// signal. Every other read answers at once.
function stubHangingWorklist(): () => AbortSignal | undefined {
  let worklist: AbortSignal | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const request = input instanceof Request ? input : new Request(input);
    if (!new URL(request.url).pathname.endsWith("/worklist")) {
      return Response.json(brief);
    }
    worklist = request.signal;
    return new Promise<Response>((_, reject) => {
      request.signal.addEventListener("abort", () =>
        reject(request.signal.reason),
      );
    });
  });
  return () => worklist;
}

it("aborts the day's Worklist read when the reader leaves before Home renders", async () => {
  const worklist = stubHangingWorklist();
  const leave = prefetchHome(createQueryClient(), new Map());
  await vi.waitFor(() => expect(worklist()).toBeDefined());

  leave();
  await vi.waitFor(() => expect(worklist()?.aborted).toBe(true));
});

it("leaves the read running when Home already watches it", async () => {
  const worklist = stubHangingWorklist();
  const client = createQueryClient();
  const leave = prefetchHome(client, new Map());
  await vi.waitFor(() => expect(worklist()).toBeDefined());
  const home = new InfiniteQueryObserver(
    client,
    worklistDayOptions("mine", "all", undefined, DEFAULT_VIEW),
  ).subscribe(() => undefined);

  leave();
  expect(worklist()?.aborted).toBe(false);
  home();
});
