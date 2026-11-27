// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./list-page-size-coverage.census";

const screen = (limit: string) => ({
  path: join(srcDir, "features", "planted.tsx"),
  area: "src" as const,
  text: `const rows = useListQuery<Row>({
  fetch: (query, cursor) => api.GET("/rows", {
    params: { query: {
      cursor: cursor || undefined,
      limit: ${limit},
    } },
  }),
});
`,
});

describe("the list-page-size-coverage rule", () => {
  it("passes a fetcher whose limit derives from the query", () => {
    expect(
      plantedFindings(rule, [screen("listFetchLimit(query.perPage)")]),
    ).toEqual([]);
  });

  it("passes a fetcher that reads at an offset through the same limit", () => {
    expect(
      plantedFindings(rule, [screen("listFetchLimit(query.perPage, reach)")]),
    ).toEqual([]);
  });

  it("reports a limit that only looks derived", () => {
    expect(
      plantedFindings(rule, [screen("listFetchLimit(query.perPage, 4)")]),
    ).toEqual([
      "src/features/planted.tsx sends the page limit listFetchLimit(query.perPage, 4)",
    ]);
  });

  it("reports a literal limit and a one-page limit", () => {
    expect(plantedFindings(rule, [screen("50")])).toEqual([
      "src/features/planted.tsx sends the page limit 50",
    ]);
    expect(plantedFindings(rule, [screen("query.perPage")])).toEqual([
      "src/features/planted.tsx sends the page limit query.perPage",
    ]);
  });

  it("reads a fetcher the screen imports from its queries module", () => {
    const planted = (limit: string) => [
      {
        path: join(srcDir, "features", "planted.tsx"),
        area: "src" as const,
        text: [
          'import { fetchRows } from "./planted.queries";',
          "const rows = useListQuery<Row>({",
          "  fetchPage: fetchRows,",
          "});",
          "",
        ].join("\n"),
      },
      {
        path: join(srcDir, "features", "planted.queries.ts"),
        area: "src" as const,
        text: [
          "export async function fetchRows(query: ListQuery, cursor: string | null) {",
          '  return api.GET("/rows", { params: { query: {',
          "    cursor: cursor || undefined,",
          `    limit: ${limit},`,
          "  } } });",
          "}",
          "",
        ].join("\n"),
      },
    ];
    expect(
      plantedFindings(rule, planted("listFetchLimit(query.perPage)")),
    ).toEqual([]);
    expect(plantedFindings(rule, planted("50"))).toEqual([
      "src/features/planted.tsx sends the page limit 50",
    ]);
  });

  it("reports a list screen with no fetcher of the shared shape", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text = "const rows = useListQuery<Row>({ fetch });\n";
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([
      "src/features/planted.tsx has no list fetcher shaped like the shared one; if the shape changed, change this check with it",
    ]);
  });
});
