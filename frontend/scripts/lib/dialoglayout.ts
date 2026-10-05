// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The corpus, layout reading and tag resolution both dialog gates share, so
 * their two readings of one tree cannot drift apart.
 */

import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { rulesIn, withoutComments } from "../../src/testing/css";
import {
  alternativesOf,
  classesOf,
  compounds,
  selectorList,
  splitTopLevel,
  subjectsOf,
} from "./css-rules";
import {
  extensionLayers,
  filesMatching,
  parseSource,
  resolveRelative,
  sourceFileAt,
} from "./source-tree";

export const srcDir = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "src",
);
const extensionsDir = join(srcDir, "..", "..", "extensions");
const underTree = (pattern: RegExp) =>
  filesMatching(srcDir, pattern).concat(
    extensionLayers(extensionsDir).flatMap((l) => filesMatching(l, pattern)),
  );
export const sheetTexts = () =>
  underTree(/\.css$/).map((f) => readFileSync(f, "utf8"));
// Stories are in: a reader copies them. Tests are out: their Modals are fixtures.
export const markupFiles = () =>
  underTree(/\.(tsx|jsx)$/).filter((f) => !/\.test\.(tsx|jsx)$/.test(f));

const BOTTOM =
  /(?:^|[;\s])(?:(?:margin|padding)-(?:bottom|block-end)|border-bottom)\s*:([^;]+)/g;
const TOP =
  /(?:^|[;\s])(?:(?:margin|padding)-(?:top|block-start)|border-top)\s*:([^;]+)/g;

export type Prop =
  | "top"
  | "bottom"
  | "display"
  | "direction"
  | "rowGap"
  | "columnGap";
// A rule's layout with the ancestors it needs: `.a > .b` lays out a `.b` only
// under an `.a`, and a heavier rule there outweighs a bare `.b`.
export type Placed = {
  subject: Set<string>;
  context: Set<string>[];
  child: boolean;
  weight: number;
  order: number;
  place?: string;
  selector: string;
  props: Partial<Record<Prop, string>>;
};
export type Owners = {
  gap: Set<string>;
  band: Set<string>;
  row: Set<string>;
  top: Set<string>;
  placed: Map<string, Placed[]>;
  loose: Set<string>;
};

export const isZero = (v: string) =>
  /^(0[a-z%]*|none|auto|normal|unset|initial|inherit|revert(-layer)?)$/.test(
    v.replace(/!important/, "").trim(),
  );
const decl = (body: string, prop: string) =>
  body.match(new RegExp(`(?:^|[;\\s])${prop}\\s*:\\s*([^;]+)`))?.[1];
const display = (body: string, kind: string) =>
  new RegExp(`(^|[;\\s])display\\s*:\\s*(inline-)?${kind}\\b`).test(body);

const MARGIN =
  /(?:^|[;\s])margin(-top|-bottom|-block|-block-start|-block-end)?\s*:\s*([^;]+)/g;
function edges(body: string) {
  const out: { top?: string; bottom?: string } = {};
  for (const [, side = "", value] of body.matchAll(MARGIN)) {
    const box = splitTopLevel(value, " \t\n");
    if (["", "-block", "-top", "-block-start"].includes(side)) out.top = box[0];
    if (["-bottom", "-block-end"].includes(side)) out.bottom = box[0];
    if (side === "") out.bottom = box[2] ?? box[0];
    if (side === "-block") out.bottom = box[1] ?? box[0];
  }
  return out;
}
function layoutOf(body: string): Placed["props"] {
  const gap = splitTopLevel(decl(body, "gap") ?? "", " \t\n");
  const flow = decl(body, "flex-direction") ?? decl(body, "flex-flow");
  const props: Placed["props"] = {
    ...edges(body),
    display: decl(body, "display"),
    direction: flow && (/^\s*column/.test(flow) ? "column" : "row"),
    rowGap: decl(body, "row-gap") ?? gap[0],
    columnGap: decl(body, "column-gap") ?? gap.at(-1),
  };
  const set = Object.entries(props).filter(([, v]) => v !== undefined);
  return Object.fromEntries(set);
}

// Specificity's class column, with `:where()` weighing nothing as it does.
function weightOf(selector: string): number {
  let rest = selector;
  for (
    let at = rest.indexOf(":where(");
    at >= 0;
    at = rest.indexOf(":where(")
  ) {
    let depth = 0;
    let end = at + ":where".length;
    for (; end < rest.length; end++) {
      if (rest[end] === "(") depth++;
      if (rest[end] === ")" && --depth === 0) break;
    }
    rest = rest.slice(0, at) + rest.slice(end + 1);
  }
  const pseudo = /(?<!:):(?!(?:is|not|has|where)\()[\w-]+/g;
  return (
    (rest.match(/\.[\w-]+|\[[^\]]*\]/g)?.length ?? 0) +
    (rest.match(pseudo)?.length ?? 0)
  );
}

// A subject behind a sibling combinator or a state pseudo-class lays out an
// element only sometimes, so it neither sets nor clears anything.
function placedOf(selector: string, body: string, order: number): Placed[] {
  const props = layoutOf(body);
  if (Object.keys(props).length === 0) return [];
  return selectorList(selector).flatMap((written) => {
    const weight = weightOf(written);
    return alternativesOf(written).flatMap((alternative) => {
      const links = compounds(alternative);
      const [subject = "", place] = (links.at(-1) ?? "").split(
        /:(first-child|last-child)$/,
      );
      if (subject.includes(":") || /[+~]/.test(alternative)) return [];
      const context = links.slice(0, -1).map(classesOf);
      const child = />\s*[^\s>]+$/.test(alternative);
      const rule = { subject: classesOf(subject), context, child, weight };
      const at = { order, props, place, selector: alternative };
      const any = subject.trim() === "*";
      return rule.subject.size > 0 || any ? [{ ...rule, ...at }] : [];
    });
  });
}

// A rule under a conditional at-rule holds only on some screens, so a layout
// census counts it as neither setting nor clearing anything.
function unconditional(sheet: string): string {
  const css = withoutComments(sheet);
  let out = css;
  for (const m of css.matchAll(/@(?:media|container|supports)\b[^{]*\{/g)) {
    const start = m.index ?? 0;
    let depth = 1;
    let end = start + m[0].length;
    for (; end < css.length && depth > 0; end++) {
      if (css[end] === "{") depth++;
      if (css[end] === "}") depth--;
    }
    const blank = css.slice(start, end).replace(/[^\n]/g, " ");
    out = out.slice(0, start) + blank + out.slice(end);
  }
  return out;
}

// What a rule's body says about the box it paints, for the title gate's sets.
function traitsOf(body: string) {
  const box = (p: string) => splitTopLevel(decl(body, p) ?? "", " \t\n");
  const rowGap = decl(body, "row-gap") ?? box("gap")[0] ?? "0";
  const direction = decl(body, "flex-direction") ?? decl(body, "flex-flow");
  const column = /^\s*column/.test(direction ?? "");
  const flex = display(body, "flex");
  const set = (v: string | undefined) => v !== undefined && !isZero(v);
  const bottom = [
    ...[...body.matchAll(BOTTOM)].map((m) => m[1]),
    ...["margin-block", "padding-block"].map((p) => box(p).at(-1)),
    ...["margin", "padding"].map((p) => box(p)[box(p).length > 2 ? 2 : 0]),
  ];
  const top = [
    ...[...body.matchAll(TOP)].map((m) => m[1]),
    ...["margin-block", "padding-block", "margin", "padding"].map(
      (p) => box(p)[0],
    ),
  ];
  return {
    gap: (display(body, "grid") || (flex && column)) && !isZero(rowGap),
    band: bottom.some(set),
    top: top.some(set),
    loose: /^(absolute|fixed)/.test(decl(body, "position") ?? ""),
    row: flex && !column,
  };
}

const TRAITS: readonly ("gap" | "band" | "top" | "loose" | "row")[] = [
  "gap",
  "band",
  "top",
  "loose",
  "row",
];

function index(placed: Map<string, Placed[]>, rule: Placed) {
  for (const name of rule.subject.size > 0 ? rule.subject : ["*"]) {
    placed.set(name, [...(placed.get(name) ?? []), rule]);
  }
}

export function ownersIn(sheets: readonly string[]): Owners {
  const owners: Owners = {
    gap: new Set(),
    band: new Set(),
    row: new Set(),
    top: new Set(),
    placed: new Map(),
    loose: new Set(),
  };
  const held = sheets.flatMap((sheet) => rulesIn(unconditional(sheet)));
  for (const [order, { selector, body }] of held.entries()) {
    for (const rule of placedOf(selector, body, order)) {
      index(owners.placed, rule);
    }
  }
  for (const { selector, body } of sheets.flatMap(rulesIn)) {
    const traits = traitsOf(body);
    const names = selectorList(selector)
      .flatMap(subjectsOf)
      .flatMap((subject) => [...classesOf(subject)]);
    for (const trait of TRAITS.filter((t) => traits[t])) {
      for (const name of names) owners[trait].add(name);
    }
  }
  return owners;
}

export const opening = (n: ts.Node) => {
  if (ts.isJsxElement(n)) return n.openingElement;
  return ts.isJsxSelfClosingElement(n) ? n : undefined;
};
export const tag = (n: ts.Node) => opening(n)?.tagName.getText() ?? "";
export const attrs = (n: ts.Node) => {
  const all: readonly ts.JsxAttributeLike[] =
    opening(n)?.attributes.properties ?? [];
  return all.filter(ts.isJsxAttribute);
};
export const attr = (n: ts.Node, name: string) =>
  attrs(n).find((a) => a.name.getText() === name);
export function descendants(root: ts.Node): ts.Node[] {
  const out: ts.Node[] = [];
  const visit = (n: ts.Node) => {
    out.push(n);
    ts.forEachChild(n, visit);
  };
  ts.forEachChild(root, visit);
  return out;
}
export const elementsIn = (root: ts.Node) => descendants(root).filter(opening);
export const enclosing = (n: ts.Node) => {
  for (let p = n.parent; p; p = p.parent) if (ts.isJsxElement(p)) return p;
  return undefined;
};
export function keys(a: ts.JsxAttribute | undefined): string[] {
  const init = a?.initializer;
  const e = init && ts.isJsxExpression(init) ? init.expression : init;
  if (e && ts.isTemplateExpression(e)) {
    return e.templateSpans.map((span) => span.expression.getText());
  }
  return e ? [e.getText()] : [];
}
export function classes(n: ts.Node): string[] {
  const init = attr(n, "className")?.initializer;
  return (init ? [init, ...descendants(init)] : [])
    .flatMap((x) =>
      ts.isStringLiteral(x) || ts.isTemplateLiteralToken(x)
        ? x.text.split(/\s+/)
        : [],
    )
    .filter(Boolean);
}
const imports = (source: ts.SourceFile) =>
  source.statements.filter(ts.isImportDeclaration);
export function localNames(source: ts.SourceFile, name: string): Set<string> {
  const bound = imports(source).flatMap((d) => {
    const named = d.importClause?.namedBindings;
    if (!named || !ts.isNamedImports(named)) return [];
    const same = named.elements.filter(
      (e) => (e.propertyName ?? e.name).text === name,
    );
    return same.map((e) => e.name.text);
  });
  return new Set([name, ...bound]);
}
export function componentSource(source: ts.SourceFile, name: string) {
  const from = imports(source).find((s) =>
    new RegExp(`\\b${name}\\b`).test(s.importClause?.getText() ?? ""),
  );
  if (!from) return source;
  const spec = from.moduleSpecifier.getText().slice(1, -1);
  const base = join(dirname(source.fileName), spec);
  const file = [`${base}.tsx`, `${base}.ts`, join(base, "index.tsx")].find(
    existsSync,
  );
  return file ? parseSource(file, readFileSync(file, "utf8")) : undefined;
}

// Every name a module imports or re-exports, with where it comes from. An
// `export { A as B }` with no `from` points back into the module itself.
function namedIn(s: ts.Statement) {
  if (!ts.isImportDeclaration(s) && !ts.isExportDeclaration(s)) return [];
  const spec = s.moduleSpecifier;
  const named = ts.isImportDeclaration(s)
    ? s.importClause?.namedBindings
    : s.exportClause;
  if ((spec && !ts.isStringLiteral(spec)) || !named) return [];
  if (!ts.isNamedImports(named) && !ts.isNamedExports(named)) return [];
  const from = spec && ts.isStringLiteral(spec) ? spec.text : "";
  return named.elements.map((e): [string, { spec: string; name: string }] => [
    e.name.text,
    { spec: from, name: (e.propertyName ?? e.name).text },
  ]);
}
const bound = new WeakMap<
  ts.SourceFile,
  Map<string, { spec: string; name: string }>
>();
function bindings(source: ts.SourceFile) {
  const known =
    bound.get(source) ?? new Map(source.statements.flatMap(namedIn));
  bound.set(source, known);
  return known;
}
const declared = new WeakMap<ts.SourceFile, Map<string, ts.Node>>();
export function declarations(source: ts.SourceFile) {
  const known = declared.get(source);
  if (known) return known;
  const out = new Map<string, ts.Node>();
  for (const n of descendants(source)) {
    const named = ts.isFunctionDeclaration(n) || ts.isVariableDeclaration(n);
    if (named && n.name && !out.has(n.name.getText()))
      out.set(n.name.getText(), n);
  }
  declared.set(source, out);
  return out;
}
// Where a component tag is defined, through imports, re-exports and renames.
const defined = new WeakMap<ts.SourceFile, Map<string, ts.Node | undefined>>();
export function definition(
  source: ts.SourceFile,
  name: string,
): ts.Node | undefined {
  const known = defined.get(source) ?? new Map<string, ts.Node | undefined>();
  defined.set(source, known);
  if (!known.has(name)) known.set(name, follow(source, name, 4));
  return known.get(name);
}
function follow(
  source: ts.SourceFile,
  name: string,
  hops: number,
): ts.Node | undefined {
  const own = declarations(source).get(name);
  const link = bindings(source).get(name);
  if (own || hops === 0 || !link) return own;
  const local = link.spec === "" && link.name !== name;
  const file = local
    ? source.fileName
    : resolveRelative(source.fileName, link.spec);
  if (!file || (link.spec === "" && !local)) return undefined;
  const from = local ? source : sourceFileAt(file);
  return follow(from, link.name, hops - 1);
}
export const definedName = (def: ts.Node) =>
  (ts.isFunctionDeclaration(def) || ts.isVariableDeclaration(def)) && def.name
    ? def.name.getText()
    : "";
export const keyOf = (def: ts.Node) =>
  `${def.getSourceFile().fileName}#${definedName(def)}`;
export const within = (n: ts.Node, outer: ts.Node) =>
  n.getSourceFile() === outer.getSourceFile() &&
  outer.pos <= n.pos &&
  n.end <= outer.end;

type Args = ReadonlyMap<string, string>;
// The few expressions `modalClass` is written in: literals, its parameters,
// `===`/`!==` between them, and a conditional over those.
function evaluate(e: ts.Expression, args: Args): string | boolean | undefined {
  if (ts.isParenthesizedExpression(e)) return evaluate(e.expression, args);
  if (ts.isStringLiteral(e)) return e.text;
  if (ts.isIdentifier(e)) return args.get(e.text);
  if (ts.isConditionalExpression(e)) {
    const pick = evaluate(e.condition, args) ? e.whenTrue : e.whenFalse;
    return evaluate(pick, args);
  }
  if (!ts.isBinaryExpression(e)) return undefined;
  const same = evaluate(e.left, args) === evaluate(e.right, args);
  const kind = e.operatorToken.kind;
  if (kind === ts.SyntaxKind.EqualsEqualsEqualsToken) return same;
  return kind === ts.SyntaxKind.ExclamationEqualsEqualsToken
    ? !same
    : undefined;
}
function execute(statements: readonly ts.Statement[], args: Args): string {
  for (const s of statements) {
    if (ts.isReturnStatement(s) && s.expression) {
      return String(evaluate(s.expression, args) ?? "");
    }
    if (ts.isIfStatement(s) && evaluate(s.expression, args)) {
      const then = s.thenStatement;
      const out = execute(ts.isBlock(then) ? then.statements : [then], args);
      if (out !== "") return out;
    }
  }
  return "";
}
const presetOf = (b: ts.BindingElement): [string, string][] =>
  b.initializer && ts.isStringLiteral(b.initializer)
    ? [[b.name.getText(), b.initializer.text]]
    : [];

// The box classes `Modal` draws for a `size` and a `placement`, read off
// `modalClass` and the props' defaults in modal.tsx rather than restated here.
export function modalClasses(size?: string, placement?: string): string[] {
  const file = sourceFileAt(join(srcDir, "design-system", "modal.tsx"));
  const fn = declarations(file).get("modalClass");
  const modal = declarations(file).get("Modal");
  if (!fn || !ts.isFunctionDeclaration(fn) || !fn.body || !modal) {
    return ["modal"];
  }
  const preset = new Map(
    descendants(modal).filter(ts.isBindingElement).flatMap(presetOf),
  );
  const given = [size, placement];
  const args = new Map(
    fn.parameters.map((p, i): [string, string] => {
      const name = p.name.getText();
      return [name, given[i] ?? preset.get(name) ?? ""];
    }),
  );
  return (execute(fn.body.statements, args) || "modal").split(/\s+/);
}
