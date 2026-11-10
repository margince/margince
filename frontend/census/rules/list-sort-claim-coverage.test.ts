// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule, screenSendsSort } from "./list-sort-claim-coverage.census";

// The builder a fetcher spreads is read alone: a sibling that sends sort does
// not speak for one that does not.
describe("a fetcher's spread builder", () => {
  const queries = [
    "export function sortedParams(query: ListQuery) {",
    "  return {",
    "    sort: query.sort || undefined,",
    "  };",
    "}",
    "",
    "export function unsortedParams(query: ListQuery) {",
    "  return { q: query.q || undefined };",
    "}",
    "",
    "export function unsortingParams(query: ListQuery) {",
    "  return { sort: undefined };",
    "}",
    "",
  ].join("\n");
  const fetcherOf = (...body: string[]) =>
    [
      'import { sortedParams, unsortedParams, unsortingParams } from "./things.queries";',
      "",
      "export async function fetchThingsPage(query: ListQuery) {",
      ...body,
      "}",
      "",
      "const list = useListQuery<Thing>({",
      "    fetchPage: fetchThingsPage,",
      "});",
      "",
    ].join("\n");
  const wireCall = (query: string) =>
    `  return api.GET("/things", { params: { query: { ${query} } } });`;
  const readModule = () => queries;

  const sends = (fetcher: string) =>
    screenSendsSort("x/things.tsx", fetcher, readModule);

  it("sends sort when the builder it spreads does", () => {
    expect(
      sends(fetcherOf(wireCall("...sortedParams(query), limit: 50"))),
    ).toBe(true);
  });

  it("does not, beside a sibling builder that does", () => {
    expect(
      sends(fetcherOf(wireCall("...unsortedParams(query), limit: 50"))),
    ).toBe(false);
  });

  it("does not, when the spread sits outside the wire query", () => {
    const fetcher = fetcherOf(
      "  const extra = { ...sortedParams(query) };",
      wireCall("limit: 50"),
    );
    expect(sends(fetcher)).toBe(false);
  });

  it("does not, when a query-shaped object before the wire call holds it", () => {
    const fetcher = fetcherOf(
      "  const echo = { query: { ...sortedParams(query) } };",
      wireCall("limit: 50"),
    );
    expect(sends(fetcher)).toBe(false);
  });

  it("does not, when a query-shaped object after the wire call holds it", () => {
    const fetcher = fetcherOf(
      '  const page = await api.GET("/things", { params: { query: { limit: 50 } } });',
      "  const echo = { query: { ...sortedParams(query) } };",
      "  return { data: page.data };",
    );
    expect(sends(fetcher)).toBe(false);
  });

  it("does not, when a later key unsets the builder's sort", () => {
    expect(
      sends(
        fetcherOf(
          wireCall("...sortedParams(query), sort: undefined, limit: 50"),
        ),
      ),
    ).toBe(false);
  });

  it("does not, when a later spread unsets the builder's sort", () => {
    expect(
      sends(
        fetcherOf(
          wireCall("...sortedParams(query), ...unsortingParams(query)"),
        ),
      ),
    ).toBe(false);
  });

  it("does not, when only a supporting read beside the list read sends it", () => {
    const fetcher = fetcherOf(
      '  const meta = await api.GET("/meta", { params: { query: { ...sortedParams(query) } } });',
      '  const page = unwrap(await api.GET("/things", { params: { query: { limit: 50 } } }));',
      "  return { data: page.data, meta };",
    );
    expect(sends(fetcher)).toBe(false);
  });

  it("does, when the list read sends it beside a supporting read that does not", () => {
    const fetcher = fetcherOf(
      '  const page = unwrap(await api.GET("/things", { params: { query: { ...sortedParams(query) } } }));',
      '  const meta = await api.GET("/meta", { params: { query: { limit: 1 } } });',
      "  return { data: page.data, meta };",
    );
    expect(sends(fetcher)).toBe(true);
  });

  it("reads past a brace inside a quoted value", () => {
    expect(
      sends(fetcherOf(wireCall('search: "}", ...sortedParams(query)'))),
    ).toBe(true);
  });

  it("reads past an apostrophe in a comment inside the wire call", () => {
    const fetcher = fetcherOf(
      '  return api.GET("/things", {',
      "    /* the wire query */ // the reader's own dials",
      "    params: { query: { ...sortedParams(query), limit: 50 } },",
      "  });",
    );
    expect(sends(fetcher)).toBe(true);
  });

  it("finds the list read behind a supporting read", () => {
    const fetcher = fetcherOf(
      '  const meta = await api.GET("/meta", { params: { query: { limit: 1 } } });',
      wireCall("...sortedParams(query), limit: 50"),
    );
    expect(sends(fetcher)).toBe(true);
  });

  it("does, for a sort written into the wire query itself", () => {
    const fetcher = fetcherOf(
      '  return api.GET("/things", {',
      "    params: {",
      "      query: {",
      "        sort: query.sort || undefined,",
      "      },",
      "    },",
      "  });",
    );
    expect(sends(fetcher)).toBe(true);
  });

  it("does not, when a later spread unsets the direct sort key", () => {
    const fetcher = fetcherOf(
      '  return api.GET("/things", {',
      "    params: {",
      "      query: {",
      "        sort: query.sort || undefined,",
      "        ...unsortingParams(query),",
      "      },",
      "    },",
      "  });",
    );
    expect(sends(fetcher)).toBe(false);
  });

  it("does not, when a later key unsets the direct sort key", () => {
    const fetcher = fetcherOf(
      '  return api.GET("/things", {',
      "    params: {",
      "      query: {",
      "        sort: query.sort || undefined,",
      "        sort: undefined,",
      "      },",
      "    },",
      "  });",
    );
    expect(sends(fetcher)).toBe(false);
  });

  it("does not, for a sort written outside the wire query", () => {
    const fetcher = fetcherOf(
      "  const echo = {",
      "    sort: query.sort || undefined,",
      "  };",
      wireCall("limit: 50"),
    );
    expect(sends(fetcher)).toBe(false);
  });

  it("does, when a later key sets the sort from the query again", () => {
    expect(
      sends(
        fetcherOf(
          wireCall("...sortedParams(query), sort: query.sort || undefined"),
        ),
      ),
    ).toBe(true);
  });
});

describe("the list-sort-claim-coverage rule", () => {
  const screen = (text: string) => ({
    path: join(srcDir, "features", "things", "things.tsx"),
    area: "src" as const,
    text,
  });
  const fetcher = (query: string, claim: string) =>
    [
      "export async function fetchThingsPage(query: ListQuery) {",
      `  return api.GET("/things", { params: { query: { ${query} } } });`,
      "}",
      "",
      "const list = useListQuery<Thing>({",
      "    fetchPage: fetchThingsPage,",
      claim,
      "});",
      "",
    ].join("\n");

  it("reports a screen that names an ordering it does not send", () => {
    expect(
      plantedFindings(rule, [
        screen(fetcher("limit: 50", '    initialSort: "-created_at",')),
      ]),
    ).toEqual([
      "src/features/things/things.tsx sends no sort and names one by initialSort",
    ]);
  });

  it("passes the same claim once the sort is on the wire", () => {
    const query = "\n    sort: query.sort || undefined,\n    limit: 50";
    expect(
      plantedFindings(rule, [
        screen(fetcher(query, '    initialSort: "-created_at",')),
      ]),
    ).toEqual([]);
  });

  it("reads a spread builder from a sibling module in the tree", () => {
    const builder = {
      path: join(srcDir, "features", "things", "things.queries.ts"),
      area: "src" as const,
      text: "export function sortedParams(query: ListQuery) {\n  return {\n    sort: query.sort || undefined,\n  };\n}\n",
    };
    const text = `import { sortedParams } from "./things.queries";\n${fetcher(
      "...sortedParams(query), limit: 50",
      "    standardViews(t),",
    )}`;
    expect(plantedFindings(rule, [builder, screen(text)])).toEqual([]);
  });
});
