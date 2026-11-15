// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A cached list of records is keyed by the reader scope it was read under.
// A same-user grant change keeps record caches, so an unscoped list keeps
// serving the rows the old grants allowed.

// A query call reads a collection when it names the path, or calls a function
// of the same file that does. Such a call must spell `readerScope`.

import { join } from "node:path";
import ts from "typescript";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

export const schemaModule = join(srcDir, "api", "schema.d.ts");

// The collections whose rows the server narrows by row scope. The floors check
// each is still a path of the API.
export const COLLECTIONS = [
  "/companies",
  "/contacts",
  "/deals",
  "/leads",
  "/offer-templates",
  "/partners",
  "/products",
  "/projects",
] as const;

const QUERY_CALLS = new Set([
  "useQuery",
  "useInfiniteQuery",
  "useQueries",
  "useSuspenseQuery",
  "queryOptions",
  "infiniteQueryOptions",
]);

const COLLECTION = new RegExp(
  `"(?:${COLLECTIONS.map((path) => path.replace(/[-/]/g, "\\$&")).join("|")})"`,
);

/** The functions this file declares, by name, with their text. */
function declaredFunctions(source: ts.SourceFile): Map<string, string> {
  const functions = new Map<string, string>();
  const visit = (node: ts.Node) => {
    if (ts.isFunctionDeclaration(node) && node.name)
      functions.set(node.name.text, node.getText(source));
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer &&
      (ts.isArrowFunction(node.initializer) ||
        ts.isFunctionExpression(node.initializer))
    )
      functions.set(node.name.text, node.getText(source));
    ts.forEachChild(node, visit);
  };
  visit(source);
  return functions;
}

/** Each query call in the file that reads a record collection with no reader scope. */
export function unscopedListReads(file: CensusFile): string[] {
  const source = file.source;
  if (!source || !COLLECTION.test(file.text)) return [];
  const readers = [...declaredFunctions(source)]
    .filter(([, text]) => COLLECTION.test(text))
    .map(([name]) => new RegExp(`\\b${name}\\(`));
  const reads = (text: string) =>
    COLLECTION.test(text) || readers.some((call) => call.test(text));
  const findings: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      QUERY_CALLS.has(node.expression.text)
    ) {
      const text = node.getText(source);
      if (reads(text) && !/\breaderScope\b/.test(text)) {
        const line =
          source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
        findings.push(
          `${file.rel}:${line}: ${node.expression.text} reads a record collection under a key with no reader scope`,
        );
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return findings;
}

export const rule: CensusRule = {
  name: "list-reader-scope",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.area === "src" &&
        /\.tsx?$/.test(file.path) &&
        !file.path.endsWith(".d.ts") &&
        !/\.(test|stories)\.tsx?$/.test(file.path) &&
        !file.path.startsWith(join(srcDir, "testing")),
    ),
  floors: (corpus, tree) => {
    const schema = tree.at(schemaModule)?.text ?? "";
    const gone = COLLECTIONS.filter(
      (path) => !schema.includes(`    "${path}": {`),
    ).map((path) => `${path} is no longer a path in api/schema.d.ts`);
    const judged = corpus.filter((file) => COLLECTION.test(file.text));
    return judged.length >= 10
      ? gone
      : [...gone, "fewer than 10 modules name a record collection"];
  },
  visit: unscopedListReads,
  advice:
    "A query that reads a record collection keys its rows by the reader " +
    "scope, or a same-user grant change keeps serving the old rows. Read " +
    '`const readerScope = useHealthScope()`, put `readerScope ?? ""` in the ' +
    "key and add `readerScope !== undefined` to `enabled`, as useDeals does.",
};
