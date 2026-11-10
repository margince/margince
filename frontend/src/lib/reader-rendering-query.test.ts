// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../testing/mefixture";
import { renderingOptions } from "./reader-rendering-query";

const id = "11111111-1111-4111-8111-111111111111";
const rendering = {
  activity_id: id,
  target_language: "vi",
  source_version: 7,
  part: "subject",
  subject: "Gửi tài liệu",
  unchanged: false,
  generated_at: "2026-11-09T00:00:00Z",
};
const clients: QueryClient[] = [];
afterEach(() => {
  for (const client of clients) client.clear();
  clients.length = 0;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function client() {
  const value = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  value.setQueryData(["me"], {
    ...meFixture(),
    reader_rendering_available: true,
  });
  clients.push(value);
  return value;
}
function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

it("reuses a translation only after a fresh authorized source read without renewing its lifetime", async () => {
  const cache = client();
  let now = 1000;
  vi.spyOn(Date, "now").mockImplementation(() => now);
  const fetch = vi.fn(async (request: Request) =>
    request.method === "POST"
      ? response(rendering)
      : response({
          id,
          version: 7,
          system_display: { generated_parts: ["subject"] },
        }),
  );
  vi.stubGlobal("fetch", fetch);
  const options = renderingOptions(cache, id, 7, "vi", "subject");
  const first = await cache.fetchQuery(options);
  now = 2000;
  const second = await cache.fetchQuery(options);
  expect(second).toEqual(first);
  expect(second.renderedAt).toBe(1000);
  expect(fetch.mock.calls.map(([request]) => request.method)).toEqual([
    "POST",
    "GET",
  ]);
  expect(await fetch.mock.calls[0]?.[0].json()).toEqual({
    locale: "vi",
    part: "subject",
    expected_source_version: 7,
  });
  now = 302000;
  await cache.fetchQuery(options);
  expect(fetch.mock.calls.map(([request]) => request.method)).toEqual([
    "POST",
    "GET",
    "POST",
  ]);
});

it("refuses cached text after visibility or version changes", async () => {
  const cache = client();
  const fetch = vi.fn(async (request: Request) =>
    request.method === "POST"
      ? response(rendering)
      : response({
          id,
          version: 8,
          system_display: { generated_parts: ["subject"] },
        }),
  );
  vi.stubGlobal("fetch", fetch);
  const options = renderingOptions(cache, id, 7, "vi", "subject");
  await cache.fetchQuery(options);
  await expect(cache.fetchQuery(options)).rejects.toThrow("source changed");
  expect(fetch.mock.calls.map(([request]) => request.method)).toEqual([
    "POST",
    "GET",
  ]);
});

it("keeps locale and source revision in independent keys and disables automatic failure retries", () => {
  const cache = client();
  const original = renderingOptions(cache, id, 7, "vi", "subject");
  expect(original.queryKey).not.toEqual(
    renderingOptions(cache, id, 7, "de", "subject").queryKey,
  );
  expect(original.queryKey).not.toEqual(
    renderingOptions(cache, id, 8, "vi", "subject").queryKey,
  );
  expect(original.retry).toBe(false);
  expect(original.retryOnMount).toBe(false);
});
