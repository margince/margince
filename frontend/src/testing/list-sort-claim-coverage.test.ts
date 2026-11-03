// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { basename, dirname, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { SRC_ROOT, screenFiles } from "./screenfiles";

// Fitness function for a list that claims an ordering it cannot have.
//
// `/partners` accepts a `sort` parameter and its handler never reads it: the
// store orders by company id, which is its keyset. The screen opened on a
// tab labelled "Newest", sent `sort=-created_at`, and drew rows in uuid order.
// Nothing failed. The reader was simply told an ordering the list did not have,
// which is worse than being told none — a wrong order looks like data, and a
// missing one looks like a missing control.
//
// The tell is textual and unambiguous: a screen whose list fetcher does not put
// `sort` on the wire has no ordering to offer, so it may not name one either —
// not as `initialSort`, not as a view tab carrying `sort:`, and not through
// `standardViews`, whose tabs order by `-created_at` unless told otherwise.
//
// The census IS the directory, so a new list screen is covered the day it is
// written rather than the day somebody remembers this file. It is the same
// derivation `list-page-size-coverage.test.ts` makes over the same corpus, and
// for the same reason: the four screens that gate caught had each been missed
// by a hand-kept list.
//
// WHAT THIS DOES NOT CATCH, deliberately: the other direction — a screen that
// sends `sort` while declaring no sortable column, so the dial exists and
// nothing can reach it. Telling a COLUMN's `sort:` from a view tab's needs the
// type checker plus a rule for the shared column helpers in `recordlist.tsx`,
// which declare theirs in another file; a gate whose rules are that fiddly is
// one that gets worked around rather than fixed. That half is stated as a rule
// in `design-system/README.md` instead, where the next author reads it.

/** Every screen that reads a list through the shared wrapper. */
function listScreens(): string[] {
  return screenFiles()
    .filter((file) => {
      const source = readFileSync(resolve(SRC_ROOT, file), "utf8");
      // Calls it, rather than declaring it: listquery.tsx is the wrapper itself
      // and owns no fetcher of its own.
      return (
        /\buseListQuery</.test(source) &&
        !/export function useListQuery/.test(source)
      );
    })
    .sort();
}

/** Does this screen's list fetcher put `sort` on the wire? */
// The sort on the wire, bare or cast to the endpoint's sort type.
function sendsSort(source: string): boolean {
  return /\n\s*sort: \(?query\.sort \|\| undefined\)?(?: as \w+)?,/.test(
    source,
  );
}

// One function's text, from its declaration to the closing brace at column 0.
function functionBody(source: string, name: string): string | undefined {
  const start = new RegExp(
    `(?:^|\\n)(?:export )?(?:async )?function ${name}\\b`,
  ).exec(source)?.index;
  if (start === undefined) return undefined;
  const end = source.indexOf("\n}\n", start);
  return end === -1 ? undefined : source.slice(start, end + 3);
}

// The text from `text[from]`, an opening bracket, to the bracket closing it.
function balanced(text: string, from: number): string | undefined {
  const open = text[from];
  const close = open === "(" ? ")" : "}";
  let depth = 0;
  for (let at = from; at < text.length; at++) {
    if (text[at] === open) depth++;
    if (text[at] === close) depth--;
    if (depth === 0) return text.slice(from, at + 1);
  }
  return undefined;
}

// The `query: { … }` object inside the fetcher's `api.GET(…)` call.
function wireQuery(body: string): string | undefined {
  const call = body.indexOf("api.GET(");
  const args = call === -1 ? undefined : balanced(body, call + 7);
  const query = args?.indexOf("query: {") ?? -1;
  return args && query !== -1
    ? balanced(args, args.indexOf("{", query))
    : undefined;
}

type ReadModule = (fromFile: string, specifier: string) => string | undefined;

function readSibling(fromFile: string, specifier: string): string | undefined {
  const base = resolve(SRC_ROOT, dirname(fromFile), specifier);
  const path = [`${base}.ts`, `${base}.tsx`].find((candidate) =>
    existsSync(candidate),
  );
  return path ? readFileSync(path, "utf8") : undefined;
}

// The bodies of the builders the screen's `fetchPage` spreads into its wire
// query (`query: { ...leadsQueryParams(query) }`), each read alone from the
// sibling module it is imported from.
function spreadBuilders(
  file: string,
  source: string,
  readModule: ReadModule,
): string[] {
  const fetcher = /\bfetchPage: (\w+),/.exec(source)?.[1];
  const body = fetcher ? functionBody(source, fetcher) : undefined;
  const wire = body === undefined ? undefined : wireQuery(body);
  if (wire === undefined) return [];
  return [...wire.matchAll(/\.\.\.(\w+)\(query\)/g)].flatMap((spread) => {
    const name = spread[1];
    // A later `sort:` key that does not read the query overrides the builder's.
    const after = wire.slice(spread.index + spread[0].length);
    if (name === undefined || /\bsort:(?!\s*\(?query\.sort\b)/.test(after)) {
      return [];
    }
    const from = new RegExp(
      `import \\{[^}]*\\b${name}\\b[^}]*\\} from "(\\.[^"]+)"`,
    ).exec(source)?.[1];
    const module = from ? readModule(file, from) : undefined;
    const builder = module ? functionBody(module, name) : undefined;
    return builder ? [builder] : [];
  });
}

function screenSendsSort(
  file: string,
  source: string,
  readModule: ReadModule = readSibling,
): boolean {
  return (
    sendsSort(source) ||
    spreadBuilders(file, source, readModule).some(sendsSort)
  );
}

/** Every way this screen names an ordering to the reader. */
function sortClaims(source: string): string[] {
  const claims: string[] = [];
  if (/\n\s*initialSort:/.test(source)) {
    claims.push("initialSort");
  }
  // A view tab's own sort. `label:` beside it is what tells a tab from the
  // fetcher's `sort: query.sort` and from a column's field name.
  if (/\{\s*label: [^}]*\bsort: "/.test(source)) {
    claims.push("a view tab carrying sort:");
  }
  // Its tabs order by -created_at unless the caller names another sort, so
  // calling it is a claim whatever the arguments say.
  if (/\bstandardViews\(/.test(source)) {
    claims.push("standardViews");
  }
  return claims;
}

describe("a list claims no ordering its endpoint cannot give", () => {
  it("finds the list screens rather than trusting a list written here", () => {
    // The census IS the directory. A hand-kept list would be the thing that
    // went stale, which is the failure this test exists to prevent.
    expect(listScreens().map((file) => basename(file))).toContain(
      "contacts.tsx",
    );
    expect(listScreens().map((file) => basename(file))).toContain(
      "partners.tsx",
    );
    expect(listScreens().length).toBeGreaterThanOrEqual(6);
  });

  it.each(listScreens())("%s", (file) => {
    const source = readFileSync(resolve(SRC_ROOT, file), "utf8");
    if (screenSendsSort(file, source)) {
      return;
    }
    expect(
      sortClaims(source),
      `${file}'s list fetcher sends no sort, so the server orders these rows ` +
        `its own way — naming an ordering anyway tells the reader something ` +
        `the list does not do. Either put sort on the wire or drop the claim.`,
    ).toEqual([]);
  });
});

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
  ].join("\n");
  const fetcherOf = (...body: string[]) =>
    [
      'import { sortedParams, unsortedParams } from "./things.queries";',
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
      "  return page;",
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
