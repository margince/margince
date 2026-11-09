// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A list claims no ordering its endpoint cannot give. A wrong order looks like
// data, where a missing one looks like a missing control.
//
// A screen whose list fetcher puts no `sort` on the wire names no ordering. Not
// as `initialSort`, not as a view tab carrying `sort:`, and not through
// `standardViews`, whose tabs order by `-created_at` by default.
//
// Not caught: a screen that sends `sort` while declaring no sortable column.
// Telling a column's `sort:` from a view tab's needs the type checker, so that
// half is a rule in `design-system/README.md`.

import { basename, dirname, join, resolve } from "node:path";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  srcDir,
} from "../tree";
import { screenCorpus } from "./screenfiles.census";

// Calls the wrapper rather than declaring it: listquery.tsx owns no fetcher.
const readsAList = (file: CensusFile) =>
  /\buseListQuery</.test(file.text) &&
  !/export function useListQuery/.test(file.text);

// The sort on the wire, bare or cast to the endpoint's sort type.
const SORT_KEY = /\n\s*sort: \(?query\.sort \|\| undefined\)?(?: as \w+)?,/g;

/** Does this screen's list fetcher put `sort` on the wire? */
function sendsSort(source: string): boolean {
  return new RegExp(SORT_KEY).test(source);
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
// A bracket or quote inside a string or a comment is text, not structure.
function balanced(text: string, from: number): string | undefined {
  const open = text[from];
  const close = open === "(" ? ")" : "}";
  let depth = 0;
  let quote: string | undefined;
  for (let at = from; at < text.length; at++) {
    const char = text[at];
    if (quote !== undefined) {
      if (char === "\\") at++;
      else if (char === quote) quote = undefined;
      continue;
    }
    if (text.startsWith("//", at) || text.startsWith("/*", at)) {
      const end = text.startsWith("//", at)
        ? text.indexOf("\n", at)
        : text.indexOf("*/", at) + 1;
      if (end <= 0) return undefined;
      at = end;
      continue;
    }
    if (char === '"' || char === "'" || char === "`") quote = char;
    else if (char === open) depth++;
    else if (char === close) depth--;
    if (depth === 0) return text.slice(from, at + 1);
  }
  return undefined;
}

// The arguments of the fetcher's list read: the `api.GET(…)` it returns, or the
// one bound to the name whose `.data` it returns as its rows.
function listReadArgs(body: string): string | undefined {
  const direct = /return (?:unwrap\()?(?:await )?api\.GET\(/.exec(body);
  if (direct) return balanced(body, direct.index + direct[0].length - 1);
  const rows = [...body.matchAll(/\bdata: (\w+)\.data\b/g)].at(-1)?.[1];
  const binding = rows
    ? new RegExp(`const (?:${rows}|\\{[^}]*\\b${rows}\\b[^}]*\\}) =`).exec(body)
    : null;
  const call = binding ? body.indexOf("api.GET(", binding.index) : -1;
  if (
    !binding ||
    call === -1 ||
    body.slice(binding.index, call).includes(";")
  ) {
    return undefined;
  }
  return balanced(body, call + "api.GET".length);
}

// The `query: { … }` object of the fetcher's list read.
export function wireQuery(body: string): string | undefined {
  const args = listReadArgs(body);
  const query = args?.indexOf("query: {") ?? -1;
  return args && query !== -1
    ? balanced(args, args.indexOf("{", query))
    : undefined;
}

// The function a screen hands `useListQuery` as `fetchPage`: a named one, or
// one built in place as `const fetchPage = useMemo(…)`.
export function fetcherBody(source: string): string | undefined {
  const named = /\bfetchPage: (\w+),/.exec(source)?.[1];
  if (named !== undefined) return functionBody(source, named);
  const inline = source.indexOf("const fetchPage = ");
  const open = inline === -1 ? -1 : source.indexOf("(", inline);
  return open === -1 ? undefined : balanced(source, open);
}

export type ReadModule = (
  fromFile: string,
  specifier: string,
) => string | undefined;

// One builder's body, read alone from the sibling module the screen imports it
// from.
function builderBody(
  file: string,
  source: string,
  name: string,
  readModule: ReadModule,
): string | undefined {
  const from = new RegExp(
    `import \\{[^}]*\\b${name}\\b[^}]*\\} from "(\\.[^"]+)"`,
  ).exec(source)?.[1];
  const module = from ? readModule(file, from) : undefined;
  return module ? functionBody(module, name) : undefined;
}

// A `sort:` key that does not read the query, which unsets or replaces it.
const OVERRIDING_SORT = /\bsort:(?!\s*\(?query\.sort\b)/;

type WireSpread = { index: number; end: number; body: string | undefined };

function wireSpreads(
  file: string,
  source: string,
  wire: string,
  readModule: ReadModule,
): WireSpread[] {
  return [...wire.matchAll(/\.\.\.(\w+)\(query\)/g)].map((spread) => ({
    index: spread.index,
    end: spread.index + spread[0].length,
    body: builderBody(file, source, spread[1] ?? "", readModule),
  }));
}

// Whether a literal key, or a spread whose builder body, after `end` unsets or
// replaces the sort set before it.
function overriddenAfter(
  wire: string,
  end: number,
  spreads: WireSpread[],
): boolean {
  return (
    OVERRIDING_SORT.test(wire.slice(end)) ||
    spreads.some(
      (later) =>
        later.index >= end &&
        later.body !== undefined &&
        OVERRIDING_SORT.test(later.body),
    )
  );
}

export function screenSendsSort(
  file: string,
  source: string,
  readModule: ReadModule,
): boolean {
  const body = fetcherBody(source);
  const wire = body === undefined ? undefined : wireQuery(body);
  if (wire === undefined) return false;
  const spreads = wireSpreads(file, source, wire, readModule);
  const direct = [...wire.matchAll(SORT_KEY)].some(
    (key) => !overriddenAfter(wire, key.index + key[0].length, spreads),
  );
  const built = spreads.filter(
    (spread) =>
      spread.body !== undefined && !overriddenAfter(wire, spread.end, spreads),
  );
  return direct || built.some((spread) => sendsSort(spread.body ?? ""));
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

/** A sibling module read from the tree, by a path from src/ and a specifier. */
const treeModules =
  (tree: CensusTree): ReadModule =>
  (fromFile, specifier) => {
    const base = resolve(srcDir, dirname(fromFile), specifier);
    return (tree.at(`${base}.ts`) ?? tree.at(`${base}.tsx`))?.text;
  };

const fromSrc = (file: CensusFile) => file.rel.replace(/^src\//, "");

// The company contacts list builds its fetcher in place with useMemo.
const inPlaceFetcher = join(
  srcDir,
  "features/companies/companycontacts/contacts.tsx",
);

export const rule: CensusRule = {
  name: "list-sort-claim-coverage",
  corpus: (tree) => screenCorpus(tree).filter(readsAList),
  floors: (corpus, tree) => {
    const names = new Set(corpus.map((file) => basename(file.path)));
    const body = fetcherBody(tree.at(inPlaceFetcher)?.text ?? "");
    return [
      ...["contacts.tsx", "partners.tsx"]
        .filter((name) => !names.has(name))
        .map((name) => `${name} was not found among the list screens`),
      ...(corpus.length >= 6 ? [] : ["fewer than 6 list screens were found"]),
      ...(body?.trimEnd().endsWith("[companyId],\n  )") &&
      wireQuery(body ?? "")?.includes("sort:")
        ? []
        : ["the company contacts list's in-place fetcher was not read whole"]),
    ];
  },
  visit: (file, tree) =>
    screenSendsSort(fromSrc(file), file.text, treeModules(tree))
      ? []
      : sortClaims(file.text).map(
          (claim) => `${file.rel} sends no sort and names one by ${claim}`,
        ),
  advice:
    "A list fetcher sends no sort, so the server orders these rows its own " +
    "way, and naming an ordering anyway tells the reader something the list " +
    "does not do. Either put sort on the wire or drop the claim.",
};
