// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryClient, queryOptions } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { healthScope } from "./healthscopekey";
import { unwrap } from "./problem";

type Rendering = components["schemas"]["ActivityReaderRendering"];
type Part = "subject" | "body";
export const renderingPrefix = "reader-rendering";
const prefix = renderingPrefix;
type CachedRendering = { rendering: Rendering; renderedAt: number };
const lifetime = 5 * 60_000;
let running = 0;
const waiters: Array<() => void> = [];

async function limited<T>(
  signal: AbortSignal,
  work: () => Promise<T>,
): Promise<T> {
  if (running >= 2) await new Promise<void>((resolve) => waiters.push(resolve));
  else running++;
  try {
    signal.throwIfAborted();
    return await work();
  } finally {
    const next = waiters.shift();
    if (next) next();
    else running--;
  }
}

export function renderingOptions(
  client: QueryClient,
  id: string,
  version: number,
  locale: "en" | "de" | "vi",
  part: Part,
) {
  const me = client.getQueryData<components["schemas"]["MeResponse"]>(["me"]);
  const scope = healthScope(me);
  const queryKey = [prefix, scope, id, version, locale, part, 1] as const;
  return queryOptions({
    queryKey,
    enabled: scope !== undefined && me?.reader_rendering_available === true,
    retry: false,
    retryOnMount: false,
    staleTime: 0,
    gcTime: lifetime,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    queryFn: ({ signal }) =>
      limited(signal, async () => {
        const cached = client.getQueryData<CachedRendering>(queryKey);
        if (cached && Date.now() - cached.renderedAt < lifetime) {
          const current = unwrap(
            await api.GET("/activities/{id}", {
              params: { path: { id } },
              signal,
            }),
          );
          if (
            current.version !== version ||
            current.content_state === "withheld" ||
            !current.system_display?.generated_parts?.includes(part)
          ) {
            throw new Error("The source changed. Refresh the activity.");
          }
          return cached;
        }
        if (
          client.getQueryCache().findAll({ queryKey: [prefix], type: "active" })
            .length > 128
        )
          throw new Error("Translation cache is full.");
        const rendering = unwrap(
          await api.POST("/activities/{id}/reader-rendering", {
            params: { path: { id } },
            body: { locale, part, expected_source_version: version },
            signal,
          }),
        );
        const candidate = { rendering, renderedAt: Date.now() };
        const bytes = client
          .getQueryCache()
          .findAll({ queryKey: [prefix] })
          .reduce(
            (total, entry) =>
              total +
              new TextEncoder().encode(JSON.stringify(entry.state.data ?? null))
                .byteLength,
            0,
          );
        if (
          bytes +
            new TextEncoder().encode(JSON.stringify(candidate)).byteLength >
          2 * 1024 * 1024
        )
          throw new Error("Translation cache is full.");
        return candidate;
      }),
  });
}

export function boundRenderingCache(client: QueryClient): void {
  const entries = client
    .getQueryCache()
    .findAll({ queryKey: [prefix] })
    .sort((a, b) => b.state.dataUpdatedAt - a.state.dataUpdatedAt);
  let bytes = 0;
  for (const [index, entry] of entries.entries()) {
    const size = new TextEncoder().encode(
      JSON.stringify(entry.state.data ?? null),
    ).byteLength;
    bytes += size;
    if (!entry.isActive() && (index >= 128 || bytes > 2 * 1024 * 1024))
      client.removeQueries({ queryKey: entry.queryKey, exact: true });
  }
}
