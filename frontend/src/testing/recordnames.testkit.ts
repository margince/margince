// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { RouteMap } from "./story-utils";

type Names = Readonly<Record<string, Readonly<Record<string, string>>>>;

export function recordNamesAnswer(url: URL, names: Names): Response | null {
  if (url.pathname !== "/v1/records/references") return null;
  const kind = url.searchParams.get("kind") ?? "";
  return Response.json({
    data: url.searchParams.getAll("id").flatMap((id) => {
      const name = names[kind]?.[id];
      return name === undefined ? [] : [{ id, name, archived: false }];
    }),
  });
}

export function recordNamesRoutes(names: Names) {
  return {
    "GET /records/references": (_body: unknown, url: URL) =>
      recordNamesAnswer(url, names) ?? Response.json({ data: [] }),
  } satisfies RouteMap;
}
