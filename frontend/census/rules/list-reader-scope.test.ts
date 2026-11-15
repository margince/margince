// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { COLLECTIONS, rule, schemaModule } from "./list-reader-scope.census";

const planted = (text: string, name = "planted.tsx") =>
  plantedFindings(rule, [
    { path: join(srcDir, "features", name), area: "src", text },
  ]);

describe("the list-reader-scope rule", () => {
  it("reports a list read whose key carries no reader scope", () => {
    expect(
      planted(`export function useOptions() {
  return useQuery({
    queryKey: ["companies", "options"],
    queryFn: () => api.GET("/companies", {}),
  });
}
`),
    ).toEqual([
      "src/features/planted.tsx:2: useQuery reads a record collection under a key with no reader scope",
    ]);
  });

  it("follows a read into a function of the same file", () => {
    expect(
      planted(`async function readPage(cursor) {
  return api.GET("/deals", { params: { query: { cursor } } });
}
export function useRows() {
  return useInfiniteQuery({
    queryKey: ["deals"],
    queryFn: ({ pageParam }) => readPage(pageParam),
  });
}
`),
    ).toHaveLength(1);
  });

  it("reports an imperative read through the client", () => {
    expect(
      planted(`export function page(client, cursor) {
  return client.fetchQuery({
    queryKey: ["deals", cursor],
    queryFn: () => api.GET("/deals", { params: { query: { cursor } } }),
  });
}
`),
    ).toEqual([
      "src/features/planted.tsx:2: fetchQuery reads a record collection under a key with no reader scope",
    ]);
  });

  it("passes a read keyed by the reader scope", () => {
    expect(
      planted(`export function useOptions() {
  const readerScope = useHealthScope();
  return useQuery({
    queryKey: ["companies", readerScope ?? ""],
    enabled: readerScope !== undefined,
    queryFn: () => api.GET("/companies", {}),
  });
}
`),
    ).toEqual([]);
  });

  it("does not judge a read of one record", () => {
    expect(
      planted(`export function useCompany(id) {
  return useQuery({
    queryKey: ["company", id],
    queryFn: () => api.GET("/companies/{id}", { params: { path: { id } } }),
  });
}
`),
    ).toEqual([]);
  });

  it("does not judge a test", () => {
    expect(
      planted(
        `useQuery({ queryKey: ["x"], queryFn: () => api.GET("/leads") });\n`,
        "planted.test.tsx",
      ),
    ).toEqual([]);
  });

  it("fails its floor when a collection leaves the API", () => {
    const schema = COLLECTIONS.slice(1)
      .map((path) => `    "${path}": {`)
      .join("\n");
    const tree = treeOf([{ path: schemaModule, area: "src", text: schema }]);
    expect(rule.floors?.(rule.corpus(tree), tree)).toContain(
      `${COLLECTIONS[0]} is no longer a path in api/schema.d.ts`,
    );
  });
});
