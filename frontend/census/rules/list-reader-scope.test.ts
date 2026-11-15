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

const NO_KEY =
  "src/features/planted.tsx:3: useQuery reads a record collection, but its key carries no reader scope";
const NO_WAIT =
  "src/features/planted.tsx:3: useQuery reads a record collection, but it does not wait for the reader scope";

describe("the list-reader-scope rule", () => {
  it("reports a list read with neither a scoped key nor a wait", () => {
    expect(
      planted(`export function useOptions() {
  const readerScope = useHealthScope();
  return useQuery({
    queryKey: ["companies", "options"],
    queryFn: () => api.GET("/companies", {}),
  });
}
`),
    ).toEqual([NO_KEY, NO_WAIT]);
  });

  it("reports a scoped key whose read does not wait for the scope", () => {
    expect(
      planted(`function useProjectPage(companyId, enabled) {
  const readerScope = useHealthScope();
  return useQuery({
    queryKey: ["projects", "open", companyId, readerScope ?? ""],
    queryFn: () => api.GET("/projects", {}),
    enabled,
  });
}
`),
    ).toEqual([NO_WAIT]);
  });

  it("reports a wait whose key carries no scope", () => {
    expect(
      planted(`export function useOptions() {
  const readerScope = useHealthScope();
  return useQuery({
    queryKey: ["companies"],
    enabled: readerScope !== undefined,
    queryFn: () => api.GET("/companies", {}),
  });
}
`),
    ).toEqual([NO_KEY]);
  });

  it("passes a scoped key that waits through enabled or skipToken", () => {
    expect(
      planted(`export function useOptions() {
  const readerScope = useHealthScope();
  return useQuery({
    queryKey: ["companies", readerScope ?? ""],
    enabled: readerScope !== undefined,
    queryFn: () => api.GET("/companies", {}),
  });
}
export function useOthers() {
  const readerScope = useHealthScope();
  return useQuery({
    queryKey: ["deals", readerScope ?? ""],
    queryFn: readerScope === undefined ? skipToken : () => api.GET("/deals", {}),
  });
}
`),
    ).toEqual([]);
  });

  it("follows helpers through more than one step", () => {
    expect(
      planted(`async function readOne(cursor) {
  return api.GET("/deals", { params: { query: { cursor } } });
}
const readPage = (cursor) => readOne(cursor);
export function useRows() {
  return useInfiniteQuery({
    queryKey: ["deals"],
    queryFn: ({ pageParam }) => readPage(pageParam),
  });
}
`),
    ).toHaveLength(2);
  });

  it("resolves a path read out of a table", () => {
    expect(
      planted(`const GROUPS = { contact: { path: "/contacts" } };
export function useTagged(kind) {
  const group = GROUPS[kind];
  return useQuery({
    queryKey: ["tag-records", kind],
    queryFn: () => api.GET(group.path, {}),
  });
}
`),
    ).toHaveLength(2);
  });

  it("does not follow a name declared in another function", () => {
    expect(
      planted(`function other() {
  const data = api.GET("/contacts", {});
  return data;
}
export function useMine(id) {
  return useQuery({
    queryKey: ["company", id],
    queryFn: async () => (await api.GET("/companies/{id}", {})).data,
  });
}
`),
    ).toEqual([]);
  });

  it("reports an imperative read's key but leaves its wait to the caller", () => {
    expect(
      planted(`export function page(client, cursor) {
  return client.fetchQuery({
    queryKey: ["deals", cursor],
    queryFn: () => api.GET("/deals", { params: { query: { cursor } } }),
  });
}
`),
    ).toEqual([
      "src/features/planted.tsx:2: fetchQuery reads a record collection, but its key carries no reader scope",
    ]);
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
