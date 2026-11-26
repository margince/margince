// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The MCP app views ship English outside the i18n catalog. Their copy is held
// to the checks src/i18n/copy-style.test.ts runs, read from that file's syntax.

import { join, sep } from "node:path";
import ts from "typescript";
import { type CensusFile, type CensusRule, srcDir } from "../tree";
import { englishGate } from "./copy-style-vi.census";

const viewsDir = join(srcDir, "mcp-apps") + sep;

// Tests, fixtures and Storybook hosts are not drawn by a shipped view.
const NOT_SHIPPED =
  /\.(test|stories|testkit)\.tsx?$|\/fixture\.ts$|\/story-hosts\.tsx$/;

const PAGE_TITLE = /<title>([^<]*)<\/title>/g;
const PAGE_LABEL = /\b(aria-label|alt|placeholder|title)="([^"]*)"/g;

/** One piece of view copy: where it stands and its text. A template's
 *  substitutions read as `{}`, the catalog's placeholder. */
export type Copy = Readonly<{ at: string; text: string }>;

/** One check of the English gate: its title, its pattern and the text it
 *  reads of a value. */
export type Check = Readonly<{
  title: string;
  pattern: RegExp;
  text: (value: string) => string;
}>;

const isPage = (file: CensusFile) => file.path.endsWith("/index.html");

const shipped = (file: CensusFile) =>
  file.path.startsWith(viewsDir) &&
  (isPage(file) || (file.source !== undefined && !NOT_SHIPPED.test(file.path)));

function literalText(node: ts.Node): string | undefined {
  if (ts.isJsxText(node)) {
    const text = node.text.trim();
    return text === "" ? undefined : text;
  }
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    const module =
      ts.isImportDeclaration(node.parent) ||
      ts.isExportDeclaration(node.parent);
    return module ? undefined : node.text;
  }
  if (ts.isTemplateExpression(node)) {
    return (
      node.head.text +
      node.templateSpans.map((span) => `{}${span.literal.text}`).join("")
    );
  }
  return undefined;
}

/** copyOf is every string a view file shows: a module's literals and JSX
 *  text, a page's title and its labels. */
export function copyOf(file: CensusFile): Copy[] {
  const found: Copy[] = [];
  if (file.source === undefined) {
    for (const title of file.text.matchAll(PAGE_TITLE)) {
      found.push({ at: `${file.rel}:<title>`, text: title[1] ?? "" });
    }
    for (const label of file.text.matchAll(PAGE_LABEL)) {
      const line = file.text.slice(0, label.index).split("\n").length;
      found.push({
        at: `${file.rel}:${line}:${label[1]}`,
        text: label[2] ?? "",
      });
    }
    return found;
  }
  const source = file.source;
  const visit = (node: ts.Node): void => {
    const text = literalText(node);
    if (text !== undefined) {
      const { line, character } = source.getLineAndCharacterOfPosition(
        node.getStart(source),
      );
      found.push({ at: `${file.rel}:${line + 1}:${character + 1}`, text });
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const calleeIs = (node: ts.Node, name: string): node is ts.CallExpression =>
  ts.isCallExpression(node) &&
  ts.isIdentifier(node.expression) &&
  node.expression.text === name;

/** The statement declaring `name`, in the check's own body first. */
function declarationOf(
  name: string,
  scopes: readonly ts.Node[],
): ts.Statement | undefined {
  for (const scope of scopes) {
    let found: ts.Statement | undefined;
    const visit = (node: ts.Node): void => {
      if (found) return;
      if (ts.isFunctionDeclaration(node) && node.name?.text === name) {
        found = node;
      } else if (
        ts.isVariableStatement(node) &&
        node.declarationList.declarations.some(
          (d) => ts.isIdentifier(d.name) && d.name.text === name,
        )
      ) {
        found = node;
      } else {
        ts.forEachChild(node, visit);
      }
    };
    visit(scope);
    if (found) return found;
  }
  return undefined;
}

/** The value the gate binds to `name`, run from the gate's own code so the
 *  rule holds no second copy of a pattern or a helper. */
function bound(name: string, scopes: readonly ts.Node[]): unknown {
  const declaration = declarationOf(name, scopes);
  if (!declaration) {
    throw new Error(`${englishGate} declares no ${name}; this rule reads it`);
  }
  const js = ts.transpileModule(declaration.getText(), {
    compilerOptions: { target: ts.ScriptTarget.ES2022 },
  }).outputText;
  return new Function(`${js}\nreturn ${name};`)();
}

function expressionValue(
  node: ts.Expression,
  scopes: readonly ts.Node[],
): unknown {
  if (ts.isRegularExpressionLiteral(node)) {
    const end = node.text.lastIndexOf("/");
    return new RegExp(node.text.slice(1, end), node.text.slice(end + 1));
  }
  if (ts.isIdentifier(node)) return bound(node.text, scopes);
  return undefined;
}

/** The patterns an `offenders(…)` call checks: its own, or one per word when
 *  it runs inside `LIST.flatMap((word) => offenders(build(word), …))`. */
function patternsOf(
  call: ts.CallExpression,
  scopes: readonly ts.Node[],
): RegExp[] | undefined {
  const [first] = call.arguments;
  if (!first) return undefined;
  const direct = expressionValue(first, scopes);
  if (direct instanceof RegExp) return [direct];
  const mapper = call.parent;
  const loop = mapper.parent;
  if (
    !ts.isCallExpression(first) ||
    !ts.isIdentifier(first.expression) ||
    !ts.isArrowFunction(mapper) ||
    !ts.isCallExpression(loop) ||
    !ts.isPropertyAccessExpression(loop.expression)
  ) {
    return undefined;
  }
  const list = expressionValue(loop.expression.expression, scopes);
  const build = bound(first.expression.text, scopes);
  if (!Array.isArray(list) || typeof build !== "function") return undefined;
  const built: unknown[] = list.map((item) => build(item));
  return built.every((pattern) => pattern instanceof RegExp)
    ? built
    : undefined;
}

function textOf(
  call: ts.CallExpression,
  scopes: readonly ts.Node[],
): (value: string) => string {
  const options = call.arguments[1];
  const text =
    options && ts.isObjectLiteralExpression(options)
      ? options.properties.find(
          (p): p is ts.PropertyAssignment | ts.ShorthandPropertyAssignment =>
            (ts.isPropertyAssignment(p) ||
              ts.isShorthandPropertyAssignment(p)) &&
            ts.isIdentifier(p.name) &&
            p.name.text === "text",
        )
      : undefined;
  if (!text) return (value) => value;
  const name = ts.isPropertyAssignment(text) ? text.initializer : text.name;
  const read = ts.isIdentifier(name) ? bound(name.text, scopes) : undefined;
  if (typeof read !== "function") {
    throw new Error(
      `${englishGate}: a check reads its text by a function this rule cannot name`,
    );
  }
  return (value) => String(read(value));
}

/** checksOf throws on a gate check it cannot read, so a new shape there
 *  fails the rule instead of shrinking it. */
export function checksOf(gate: ts.SourceFile): Check[] {
  const checks: Check[] = [];
  const visit = (node: ts.Node): void => {
    if (calleeIs(node, "it")) {
      const [title, body] = node.arguments;
      if (title && ts.isStringLiteral(title) && body) {
        const scopes = [body, gate];
        const read = (call: ts.Node): void => {
          if (calleeIs(call, "offenders")) {
            const patterns = patternsOf(call, scopes);
            if (!patterns) {
              throw new Error(
                `${englishGate}: "${title.text}" checks a pattern this rule cannot read`,
              );
            }
            const text = textOf(call, scopes);
            for (const pattern of patterns) {
              checks.push({ title: title.text, pattern, text });
            }
          }
          ts.forEachChild(call, read);
        };
        read(body);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(gate);
  return checks;
}

/** Every finding over the view copy, each led by the check it breaks. */
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

export const rule: CensusRule = {
  name: "mcp-view-copy",
  corpus: (tree) => tree.files.filter(shipped),
  floors: (corpus) => [
    ...(corpus.some(isPage) ? [] : ["reads no view page under src/mcp-apps"]),
    ...(corpus.some((file) => file.source !== undefined)
      ? []
      : ["reads no view module under src/mcp-apps"]),
  ],
  across: (corpus, tree) => {
    const gate = tree.at(englishGate)?.source;
    if (!gate) {
      throw new Error(
        `${englishGate} is not in the census tree; this rule reads it`,
      );
    }
    const checks = checksOf(gate);
    if (checks.length === 0) {
      throw new Error(`${englishGate} holds no offenders(…) check to read`);
    }
    return copyFindings(corpus.flatMap(copyOf), checks);
  },
  advice:
    "An MCP app view's copy breaks docs/reference/ui-copy-style.md. Each line " +
    "names the check, where the string stands and the value to rewrite.",
};
