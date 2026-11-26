// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The MCP app views ship English outside the i18n catalog. Their copy is held
// to the checks src/i18n/copy-style.test.ts runs, read from that file's syntax.

import { join, sep } from "node:path";
import ts from "typescript";
import { type CensusFile, type CensusRule, srcDir } from "../tree";
import { type Copy, copyOf, jsonImports } from "../view-copy";
import { englishGate } from "./copy-style-vi.census";

const viewsDir = join(srcDir, "mcp-apps") + sep;

// Tests, fixtures and Storybook hosts are not drawn by a shipped view.
const NOT_SHIPPED =
  /\.(test|stories|testkit)\.tsx?$|\/fixture\.ts$|\/story-hosts\.(tsx|css)$/;

/** Tests of the gate that check its own lists, not a value of copy. */
export const NOT_COPY_CHECKS: ReadonlySet<string> = new Set([
  "retires only words the style page's Vocabulary table retires",
]);

const TEST_MODIFIERS = new Set(["skip", "only", "concurrent", "fails"]);

export type Check = Readonly<{
  title: string;
  pattern: RegExp;
  text: (value: string) => string;
}>;

const shipped = (file: CensusFile) =>
  file.path.startsWith(viewsDir) &&
  !NOT_SHIPPED.test(file.path) &&
  (file.source !== undefined || /\.(html|css)$/.test(file.path));

const isTestBase = (node: ts.Expression): boolean =>
  ts.isIdentifier(node)
    ? node.text === "it" || node.text === "test"
    : ts.isPropertyAccessExpression(node) &&
      TEST_MODIFIERS.has(node.name.text) &&
      isTestBase(node.expression);

const isTestCall = (call: ts.CallExpression): boolean =>
  isTestBase(call.expression) ||
  (ts.isCallExpression(call.expression) &&
    ts.isPropertyAccessExpression(call.expression.expression) &&
    call.expression.expression.name.text === "each" &&
    isTestBase(call.expression.expression.expression));

const calleeIs = (node: ts.Node, name: string): node is ts.CallExpression =>
  ts.isCallExpression(node) &&
  ts.isIdentifier(node.expression) &&
  node.expression.text === name;

type Scope = Readonly<{ gate: ts.SourceFile; body: ts.Node; title: string }>;

const declares = (statement: ts.Node, name: string): boolean =>
  (ts.isFunctionDeclaration(statement) && statement.name?.text === name) ||
  (ts.isVariableStatement(statement) &&
    statement.declarationList.declarations.some(
      (d) => ts.isIdentifier(d.name) && d.name.text === name,
    ));

/** A name resolves in the check's own body, then among the gate's top-level
 *  statements; a declaration in another check never answers for it. */
function declarationOf(name: string, scope: Scope): ts.Node | undefined {
  let found: ts.Node | undefined;
  const visit = (node: ts.Node): void => {
    if (found) return;
    if (declares(node, name)) found = node;
    else ts.forEachChild(node, visit);
  };
  visit(scope.body);
  return found ?? scope.gate.statements.find((s) => declares(s, name));
}

function failure(scope: Scope, what: string): Error {
  return new Error(`${englishGate}: the check "${scope.title}" ${what}`);
}

function guarded<T>(scope: Scope, name: string, run: () => T): T {
  try {
    return run();
  } catch (error) {
    if (!(error instanceof ReferenceError)) throw error;
    const missing = /^(\S+) is not defined/.exec(error.message)?.[1];
    throw failure(
      scope,
      `reads ${missing ?? error.message} through ${name}, which this rule cannot bind`,
    );
  }
}

/** The value the gate binds to `name`, run from the gate's own code so the
 *  rule holds no second copy of a pattern or a helper. */
function bound(name: string, scope: Scope): unknown {
  const declaration = declarationOf(name, scope);
  if (!declaration)
    throw failure(scope, `reads ${name}, which it does not declare`);
  const js = ts.transpileModule(declaration.getText(), {
    compilerOptions: { target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const value = guarded(scope, name, () =>
    new Function(`${js}\nreturn ${name};`)(),
  );
  return typeof value === "function"
    ? (...args: unknown[]) => guarded(scope, name, () => value(...args))
    : value;
}

const statelessCopy = (pattern: RegExp) =>
  new RegExp(pattern.source, pattern.flags.replace(/[gy]/g, ""));

function patternValue(node: ts.Expression, scope: Scope): unknown {
  if (ts.isRegularExpressionLiteral(node)) {
    const end = node.text.lastIndexOf("/");
    return new RegExp(node.text.slice(1, end), node.text.slice(end + 1));
  }
  return ts.isIdentifier(node) ? bound(node.text, scope) : undefined;
}

/** The patterns an `offenders(…)` call checks: its own, or one per word when
 *  it runs inside `LIST.flatMap((word) => offenders(build(word), …))`. */
function patternsOf(call: ts.CallExpression, scope: Scope): RegExp[] {
  const [first] = call.arguments;
  const direct = first ? patternValue(first, scope) : undefined;
  if (direct instanceof RegExp) return [direct];
  const mapper = call.parent;
  const loop = mapper.parent;
  if (
    first &&
    ts.isCallExpression(first) &&
    ts.isIdentifier(first.expression) &&
    ts.isArrowFunction(mapper) &&
    ts.isCallExpression(loop) &&
    ts.isPropertyAccessExpression(loop.expression)
  ) {
    const list = patternValue(loop.expression.expression, scope);
    const build = bound(first.expression.text, scope);
    if (Array.isArray(list) && typeof build === "function") {
      const built: unknown[] = list.map((item) => build(item));
      if (built.every((pattern) => pattern instanceof RegExp)) return built;
    }
  }
  throw failure(scope, "checks a pattern this rule cannot read");
}

function textOf(call: ts.CallExpression, scope: Scope): (v: string) => string {
  const options = call.arguments[1];
  if (!options) return (value) => value;
  if (
    !ts.isObjectLiteralExpression(options) ||
    options.properties.some(ts.isSpreadAssignment)
  ) {
    throw failure(scope, "passes options this rule cannot read");
  }
  const text = options.properties.find(
    (p) =>
      p.name !== undefined && ts.isIdentifier(p.name) && p.name.text === "text",
  );
  if (!text) return (value) => value;
  const name = ts.isPropertyAssignment(text)
    ? text.initializer
    : ts.isShorthandPropertyAssignment(text)
      ? text.name
      : undefined;
  const read =
    name && ts.isIdentifier(name) ? bound(name.text, scope) : undefined;
  if (typeof read !== "function") {
    throw failure(scope, "reads its text by a function this rule cannot name");
  }
  read("");
  return (value) => String(read(value));
}

const titleOf = (node: ts.Expression | undefined): string =>
  node && ts.isStringLiteralLike(node) ? node.text : (node?.getText() ?? "");

/** A test that gives no check, or an `offenders` outside a test, throws, so
 *  the gate cannot grow a check this rule skips. */
export function checksOf(gate: ts.SourceFile): Check[] {
  const checks: Check[] = [];
  const read = new Set<ts.Node>();
  const titles: string[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isCallExpression(node) && isTestCall(node)) {
      const [title, body = node] = node.arguments;
      const scope: Scope = { gate, body, title: titleOf(title) };
      titles.push(scope.title);
      const before = checks.length;
      const inBody = (child: ts.Node): void => {
        if (calleeIs(child, "offenders")) {
          read.add(child.expression);
          const text = textOf(child, scope);
          for (const pattern of patternsOf(child, scope)) {
            checks.push({
              title: scope.title,
              pattern: statelessCopy(pattern),
              text,
            });
          }
        }
        ts.forEachChild(child, inBody);
      };
      inBody(body);
      if (checks.length === before && !NOT_COPY_CHECKS.has(scope.title)) {
        throw failure(scope, "gives no offenders(…) check this rule can read");
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(gate);
  const unread = (node: ts.Node): string[] => [
    ...(ts.isIdentifier(node) &&
    node.text === "offenders" &&
    !ts.isFunctionDeclaration(node.parent) &&
    !read.has(node)
      ? [
          `line ${gate.getLineAndCharacterOfPosition(node.getStart(gate)).line + 1}`,
        ]
      : []),
    ...node.getChildren(gate).flatMap(unread),
  ];
  const stray = unread(gate);
  if (stray.length > 0) {
    throw new Error(
      `${englishGate} reaches offenders outside a check this rule reads, at ${stray.join(", ")}`,
    );
  }
  const stale = [...NOT_COPY_CHECKS].filter((title) => !titles.includes(title));
  if (stale.length > 0) {
    throw new Error(
      `${englishGate} has no test ${stale.join(", ")}; drop it from NOT_COPY_CHECKS`,
    );
  }
  return checks;
}

export function copyFindings(
  copy: readonly Copy[],
  checks: readonly Check[],
): string[] {
  return checks.flatMap(({ title, pattern, text }) =>
    copy
      .filter((piece) => pattern.test(text(piece.text)))
      .map((piece) => `${title}: ${piece.at} ${JSON.stringify(piece.text)}`),
  );
}

const reads = (corpus: readonly CensusFile[], kind: string, test: RegExp) =>
  corpus.some((file) => test.test(file.path))
    ? []
    : [`reads no view ${kind} under src/mcp-apps`];

export const rule: CensusRule = {
  name: "mcp-view-copy",
  corpus: (tree) => tree.files.filter(shipped),
  floors: (corpus) => [
    ...reads(corpus, "page", /\.html$/),
    ...reads(corpus, "module", /\.tsx?$/),
    ...reads(corpus, "sheet", /\.css$/),
  ],
  across: (corpus, tree) => {
    const gate = tree.at(englishGate)?.source;
    if (!gate) {
      throw new Error(
        `${englishGate} is not in the census tree; this rule reads it`,
      );
    }
    return [
      ...corpus
        .flatMap(jsonImports)
        .map((line) => `reads JSON this rule cannot: ${line}`),
      ...copyFindings(corpus.flatMap(copyOf), checksOf(gate)),
    ];
  },
  advice:
    "An MCP app view's copy breaks docs/reference/ui-copy-style.md. Each line " +
    "names the check, where the string stands and the value to rewrite. A " +
    "view that imports JSON names the file whose copy is not read.",
};
