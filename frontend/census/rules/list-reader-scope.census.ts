// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A cached list of records is keyed by the reader scope it was read under, and
// is not read before that scope is known. A same-user grant change keeps record
// caches, so an unscoped list keeps serving the rows the old grants allowed.

// A query call reads a collection when its text reaches a collection path
// through names declared in the same file: helpers, tables, local variables.
// A read through an import is not seen; the shared list builders take the
// scope as a required parameter, so the compiler holds those instead.

import { join } from "node:path";
import ts from "typescript";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

export const schemaModule = join(srcDir, "api", "schema.d.ts");

// The reads whose rows the server narrows by row scope. The floors check each
// is still a path of the API.
export const COLLECTIONS = [
  "/companies",
  "/contacts",
  "/deals",
  "/filters/preview",
  "/leads",
  "/offer-templates",
  "/partners",
  "/products",
  "/projects",
  "/search",
] as const;

// Hooks read on mount, so they also wait for the scope.
const HOOK_CALLS = new Set([
  "useQuery",
  "useInfiniteQuery",
  "useQueries",
  "useSuspenseQuery",
]);

// Builders and imperative reads: their key is judged, the wait is the caller's.
const KEYED_CALLS = new Set([
  "queryOptions",
  "infiniteQueryOptions",
  "fetchQuery",
  "fetchInfiniteQuery",
  "prefetchQuery",
  "prefetchInfiniteQuery",
  "ensureQueryData",
  "ensureInfiniteQueryData",
]);

const COLLECTION = new RegExp(
  `"(?:${COLLECTIONS.map((path) => path.replace(/[-/]/g, "\\$&")).join("|")})"`,
);
const SCOPE = /\breaderScope\b/;

/** The name a call is made by: `useQuery(…)` or `client.fetchQuery(…)`. */
function calledName(call: ts.CallExpression): string | undefined {
  const callee = call.expression;
  if (ts.isIdentifier(callee)) return callee.text;
  if (ts.isPropertyAccessExpression(callee)) return callee.name.text;
  return undefined;
}

/**
 * The functions and variables in scope at `node`, by name, with their text:
 * the statements of every enclosing block and of the module.
 */
function declarationsInScope(
  node: ts.Node,
  source: ts.SourceFile,
): Map<string, string[]> {
  const declared = new Map<string, string[]>();
  const add = (name: string, text: string) =>
    declared.set(name, [...(declared.get(name) ?? []), text]);
  for (let at: ts.Node | undefined = node; at; at = at.parent) {
    if (!ts.isBlock(at) && !ts.isSourceFile(at)) continue;
    for (const statement of at.statements) {
      if (ts.isFunctionDeclaration(statement) && statement.name)
        add(statement.name.text, statement.getText(source));
      if (ts.isVariableStatement(statement))
        for (const variable of statement.declarationList.declarations)
          if (ts.isIdentifier(variable.name))
            add(variable.name.text, variable.getText(source));
    }
  }
  return declared;
}

/** Whether `text` reaches a collection path through the file's own names. */
function readsCollection(
  text: string,
  declared: Map<string, string[]>,
  seen: Set<string> = new Set(),
): boolean {
  if (COLLECTION.test(text)) return true;
  for (const word of new Set(text.match(/\b[A-Za-z_$][\w$]*\b/g) ?? [])) {
    if (seen.has(word)) continue;
    const bodies = declared.get(word);
    if (!bodies) continue;
    seen.add(word);
    if (bodies.some((body) => readsCollection(body, declared, seen)))
      return true;
  }
  return false;
}

/** The text of each `name:` property in object literals under `node`. */
function propertyTexts(
  node: ts.Node,
  name: string,
  source: ts.SourceFile,
): string[] {
  const found: string[] = [];
  const visit = (child: ts.Node) => {
    if (
      ts.isPropertyAssignment(child) &&
      (ts.isIdentifier(child.name) || ts.isStringLiteral(child.name)) &&
      child.name.text === name
    )
      found.push(child.initializer.getText(source));
    ts.forEachChild(child, visit);
  };
  visit(node);
  return found;
}

/** What is wrong with one query call that reads a collection, if anything. */
function callFindings(
  call: ts.CallExpression,
  name: string,
  source: ts.SourceFile,
): string[] {
  const findings: string[] = [];
  // A key spread in from a same-file helper is judged where it is built.
  const keys = propertyTexts(call, "queryKey", source);
  const spread = call.getText(source).includes("...");
  if (keys.some((key) => !SCOPE.test(key)) || (keys.length === 0 && !spread))
    findings.push("its key carries no reader scope");
  if (HOOK_CALLS.has(name)) {
    const gates = [
      ...propertyTexts(call, "enabled", source),
      ...propertyTexts(call, "queryFn", source).filter((fn) =>
        fn.includes("skipToken"),
      ),
    ];
    if (!gates.some((gate) => SCOPE.test(gate)))
      findings.push("it does not wait for the reader scope");
  }
  return findings;
}

/** Each query call in the file that reads a collection and breaks the rule. */
export function unscopedListReads(file: CensusFile): string[] {
  const source = file.source;
  if (!source) return [];
  const findings: string[] = [];
  const visit = (node: ts.Node) => {
    const name = ts.isCallExpression(node) ? calledName(node) : undefined;
    if (
      ts.isCallExpression(node) &&
      name &&
      (HOOK_CALLS.has(name) || KEYED_CALLS.has(name)) &&
      readsCollection(node.getText(source), declarationsInScope(node, source))
    ) {
      const line =
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
      for (const finding of callFindings(node, name, source))
        findings.push(
          `${file.rel}:${line}: ${name} reads a record collection, but ${finding}`,
        );
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
    "scope and waits for it, or a same-user grant change keeps serving the " +
    "old rows. Read `const readerScope = useHealthScope()`, put " +
    '`readerScope ?? ""` in the key and `readerScope !== undefined` in ' +
    "`enabled`, as useDeals does.",
};
