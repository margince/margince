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

/** The name a call is made by: `useQuery(…)` or `client.fetchQuery(…)`. */
function calledName(call: ts.CallExpression): string | undefined {
  const callee = call.expression;
  if (ts.isIdentifier(callee)) return callee.text;
  if (ts.isPropertyAccessExpression(callee)) return callee.name.text;
  return undefined;
}

/** Each name a declaration binds, destructured names included. */
function boundNames(name: ts.BindingName): string[] {
  if (ts.isIdentifier(name)) return [name.text];
  return name.elements.flatMap((element) =>
    ts.isOmittedExpression(element) ? [] : boundNames(element.name),
  );
}

/**
 * The declaration each name refers to at `node`: the nearest enclosing one,
 * so an inner name shadows an outer one. A parameter binds with no known value.
 */
function declarationsInScope(node: ts.Node): Map<string, ts.Node | null> {
  const declared = new Map<string, ts.Node | null>();
  const bind = (name: string, value: ts.Node | null) => {
    if (!declared.has(name)) declared.set(name, value);
  };
  for (let at: ts.Node | undefined = node; at; at = at.parent) {
    if (ts.isFunctionLike(at))
      for (const parameter of at.parameters)
        for (const name of boundNames(parameter.name)) bind(name, null);
    if (!ts.isBlock(at) && !ts.isSourceFile(at)) continue;
    for (const statement of at.statements) {
      if (ts.isFunctionDeclaration(statement) && statement.name)
        bind(statement.name.text, statement);
      if (ts.isVariableStatement(statement))
        for (const variable of statement.declarationList.declarations)
          for (const name of boundNames(variable.name)) bind(name, variable);
    }
  }
  return declared;
}

/** Whether `name` names a member (`group.path`, `{ path: … }`), not a binding. */
function isMemberName(name: ts.Identifier): boolean {
  const parent = name.parent;
  return (
    (ts.isPropertyAccessExpression(parent) && parent.name === name) ||
    (ts.isPropertyAssignment(parent) && parent.name === name)
  );
}

/** Whether `node` reaches a collection path through the names in its scope. */
function readsCollection(
  node: ts.Node,
  source: ts.SourceFile,
  seen: Set<ts.Node> = new Set(),
): boolean {
  if (COLLECTION.test(node.getText(source))) return true;
  const declared = declarationsInScope(node);
  let reached = false;
  const visit = (child: ts.Node) => {
    if (reached) return;
    if (ts.isIdentifier(child) && !isMemberName(child)) {
      const target = declared.get(child.text);
      if (target && !seen.has(target)) {
        seen.add(target);
        reached = readsCollection(target, source, seen);
      }
    }
    ts.forEachChild(child, visit);
  };
  visit(node);
  return reached;
}

/** `useHealthScope()`, or `useHealthScope() ?? fallback`. */
function callsHealthScope(value: ts.Expression): boolean {
  if (
    ts.isBinaryExpression(value) &&
    value.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken
  )
    return callsHealthScope(value.left);
  return (
    ts.isCallExpression(value) &&
    ts.isIdentifier(value.expression) &&
    value.expression.text === "useHealthScope"
  );
}

/**
 * Whether `node` reads `readerScope` bound to `useHealthScope()`, or a
 * `readerScope` parameter a builder's caller passes in.
 */
function readsScope(node: ts.Node): boolean {
  if (ts.isIdentifier(node) && node.text === "readerScope") {
    const target = declarationsInScope(node).get("readerScope");
    return (
      target === null ||
      (target !== undefined &&
        ts.isVariableDeclaration(target) &&
        target.initializer !== undefined &&
        callsHealthScope(target.initializer))
    );
  }
  return ts.forEachChild(node, readsScope) ?? false;
}

/**
 * Whether `node` holds only when the scope's state is `operator` to unknown:
 * the comparison itself, or a chain that needs it (`&&` for a wait, `||` for
 * the condition of a `skipToken`).
 */
function requiresScope(node: ts.Expression, operator: ts.SyntaxKind): boolean {
  const unknown = (side: ts.Expression) =>
    (ts.isIdentifier(side) && side.text === "undefined") ||
    (ts.isStringLiteral(side) && side.text === "");
  if (ts.isParenthesizedExpression(node))
    return requiresScope(node.expression, operator);
  if (!ts.isBinaryExpression(node)) return false;
  const chain =
    operator === ts.SyntaxKind.ExclamationEqualsEqualsToken
      ? ts.SyntaxKind.AmpersandAmpersandToken
      : ts.SyntaxKind.BarBarToken;
  if (node.operatorToken.kind === chain)
    return (
      requiresScope(node.left, operator) || requiresScope(node.right, operator)
    );
  return (
    node.operatorToken.kind === operator &&
    ((readsScope(node.left) && unknown(node.right)) ||
      (readsScope(node.right) && unknown(node.left)))
  );
}

function property(
  options: ts.ObjectLiteralExpression,
  name: string,
): ts.Expression | undefined {
  for (const member of options.properties)
    if (
      ts.isPropertyAssignment(member) &&
      (ts.isIdentifier(member.name) || ts.isStringLiteral(member.name)) &&
      member.name.text === name
    )
      return member.initializer;
  return undefined;
}

/** The options objects a call passes: one, or each entry of `useQueries`. */
function optionsOf(call: ts.CallExpression, name: string): ts.Expression[] {
  if (name !== "useQueries") return call.arguments.slice(0, 1);
  const entries: ts.Expression[] = [];
  const visit = (child: ts.Node) => {
    if (
      ts.isObjectLiteralExpression(child) &&
      (property(child, "queryKey") ||
        child.properties.some(ts.isSpreadAssignment))
    )
      entries.push(child);
    else ts.forEachChild(child, visit);
  };
  visit(call);
  return entries;
}

/**
 * Whether the key of `options` reads the scope. A spread of a builder call
 * must pass the scope in; a spread of a name is judged at its declaration.
 */
function keyReadsScope(options: ts.Expression): boolean {
  if (ts.isCallExpression(options)) return options.arguments.some(readsScope);
  if (ts.isIdentifier(options)) {
    const target = declarationsInScope(options).get(options.text);
    return target != null && ts.isVariableDeclaration(target)
      ? target.initializer !== undefined && keyReadsScope(target.initializer)
      : false;
  }
  if (!ts.isObjectLiteralExpression(options)) return false;
  const key = property(options, "queryKey");
  if (key) return readsScope(key);
  return options.properties.some(
    (member) =>
      ts.isSpreadAssignment(member) && keyReadsScope(member.expression),
  );
}

/** Whether a hook's `options` hold the read until the scope is known. */
function hookWaits(options: ts.Expression): boolean {
  if (!ts.isObjectLiteralExpression(options)) return false;
  const enabled = property(options, "enabled");
  if (
    enabled &&
    requiresScope(enabled, ts.SyntaxKind.ExclamationEqualsEqualsToken)
  )
    return true;
  const queryFn = property(options, "queryFn");
  return (
    queryFn !== undefined &&
    ts.isConditionalExpression(queryFn) &&
    ts.isIdentifier(queryFn.whenTrue) &&
    queryFn.whenTrue.text === "skipToken" &&
    requiresScope(queryFn.condition, ts.SyntaxKind.EqualsEqualsEqualsToken)
  );
}

/** What is wrong with one query call that reads a collection, if anything. */
function callFindings(call: ts.CallExpression, name: string): string[] {
  const options = optionsOf(call, name);
  const findings: string[] = [];
  if (options.length === 0 || !options.every(keyReadsScope))
    findings.push("its key carries no reader scope");
  if (HOOK_CALLS.has(name) && !(options.length > 0 && options.every(hookWaits)))
    findings.push("it does not wait for the reader scope");
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
      readsCollection(node, source)
    ) {
      const line =
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
      for (const finding of callFindings(node, name))
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
