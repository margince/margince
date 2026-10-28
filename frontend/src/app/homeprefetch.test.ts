// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it, vi } from "vitest";
import { morningBriefOptions } from "../features/brief/brief.queries";
import { prefetchHome } from "./homeprefetch";
import { createQueryClient, liveOnMount } from "./queryclient";

afterEach(() => vi.restoreAllMocks());

const brief = { generated_at: "2026-10-28T06:00:00Z", items: [] };

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
