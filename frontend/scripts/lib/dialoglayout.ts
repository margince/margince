// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The corpus, layout reading, tag resolution and dialog set that
 * dialogtitle.test.ts and dialogrows.test.ts share.
 */

import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { classVariants } from "../../src/testing/classnames";
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

type Edges = { top?: string; bottom?: string };
// `!important` marks the declaration, so every side it fans out to keeps it.
function boxOf(value: string): string[] {
  const important = /!\s*important/.test(value);
  const v = splitTopLevel(value.replace(/!\s*important/, ""), " \t\n");
  return important ? v.map((side) => `${side} !important`) : v;
}
const SIDE =
  /(?:^|[;\s])(margin|padding)(-top|-bottom|-block|-block-start|-block-end)?\s*:\s*([^;]+)/g;
// In source order, so a later shorthand overrides an earlier longhand.
function sides(body: string, box: "margin" | "padding"): Edges {
  const out: Edges = {};
  for (const [, prop, side = "", value] of body.matchAll(SIDE)) {
    if (prop !== box) continue;
    const v = boxOf(value);
    if (["", "-block", "-top", "-block-start"].includes(side)) out.top = v[0];
    if (["-bottom", "-block-end"].includes(side)) out.bottom = v[0];
    if (side === "") out.bottom = v[2] ?? v[0];
    if (side === "-block") out.bottom = v[1] ?? v[0];
  }
  return out;
}
const BORDER =
  /(?:^|[;\s])border-(top|bottom|block-start|block-end)\s*:\s*([^;]+)/g;
function borders(body: string): Edges {
  const out: Edges = {};
  for (const [, side, value] of body.matchAll(BORDER)) {
    out[/top|start/.test(side) ? "top" : "bottom"] = value;
  }
  return out;
}
const GAP = /(?:^|[;\s])(row-gap|column-gap|gap)\s*:\s*([^;]+)/g;
function gaps(body: string) {
  const out: { rowGap?: string; columnGap?: string } = {};
  for (const [, prop, value] of body.matchAll(GAP)) {
    const box = boxOf(value);
    if (prop !== "column-gap") out.rowGap = box[0];
    if (prop !== "row-gap") out.columnGap = box.at(-1);
  }
  return out;
}
function layoutOf(body: string): Placed["props"] {
  const flow = decl(body, "flex-direction") ?? decl(body, "flex-flow");
  const props: Placed["props"] = {
    ...sides(body, "margin"),
    ...gaps(body),
    display: decl(body, "display"),
    direction: flow && (/^\s*column/.test(flow) ? "column" : "row"),
  };
  const set = Object.entries(props).filter(([, v]) => v !== undefined);
  return Object.fromEntries(set);
}

/** How a box with these props lays out its children: a gap `down` between
 * stacked rows, or `across` between ones side by side. */
export function flowOf(props: Placed["props"]) {
  const shown = (props.display ?? "").replace("!important", "");
  const flex = /flex\b/.test(shown);
  const column = props.direction?.trim() === "column";
  const gap = (v?: string) => !isZero(v || "0");
  return {
    row: flex && !column,
    down: (/grid\b/.test(shown) || (flex && column)) && gap(props.rowGap),
    across: flex && !column && gap(props.columnGap),
  };
}

const ID = 1e6;
const CLASS = 1e3;
const TYPE = 1;
const LEGACY_ELEMENT = /^(before|after|first-line|first-letter)$/;
function pseudoWeight(name: string, args: string | undefined): number {
  if (LEGACY_ELEMENT.test(name)) return TYPE;
  if (name === "where") return 0;
  const heaviest = (list: string) =>
    Math.max(0, ...selectorList(list).map(weightOf));
  if (args !== undefined && /^(is|not|has|matches|-webkit-any)$/.test(name)) {
    return heaviest(args);
  }
  const of = args && /^nth-(last-)?(child|of-type)$/.test(name);
  const among = of ? /\bof\s+([\s\S]+)$/.exec(args)?.[1] : undefined;
  return CLASS + (among ? heaviest(among) : 0);
}
function closing(text: string, from: number): number {
  let end = from;
  for (let depth = 1; depth > 0 && end < text.length; end++) {
    if (text[end] === "(") depth++;
    if (text[end] === ")") depth--;
  }
  return end;
}
const SIMPLE =
  /^(?:#(?:[\w-]|\\.)+|\.(?:[\w-]|\\.)+|\[[^\]]*\]|[a-zA-Z_][\w-]*)/;
// One simple selector's length and weight, from `at`.
function tokenAt(selector: string, at: number): [number, number] {
  const rest = selector.slice(at);
  const simple = SIMPLE.exec(rest)?.[0];
  if (simple) {
    const weight = /^[.[]/.test(simple) ? CLASS : TYPE;
    return [simple.length, simple[0] === "#" ? ID : weight];
  }
  const pseudo = /^(::?)([\w-]+)(\()?/.exec(rest);
  if (!pseudo) return [1, 0];
  const end = pseudo[3] ? closing(rest, pseudo[0].length) : pseudo[0].length;
  const args = pseudo[3] ? rest.slice(pseudo[0].length, end - 1) : undefined;
  const element = pseudo[1] === "::";
  return [end, element ? TYPE : pseudoWeight(pseudo[2].toLowerCase(), args)];
}
/** Specificity as one number: ids, then classes, attributes and
 * pseudo-classes, then types and pseudo-elements. */
export function weightOf(selector: string): number {
  let weight = 0;
  for (let at = 0; at < selector.length; ) {
    const [length, w] = tokenAt(selector, at);
    at += length;
    weight += w;
  }
  return weight;
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
  const margin = layoutOf(body);
  const [padding, border] = [sides(body, "padding"), borders(body)];
  const set = (v: string | undefined) => v !== undefined && !isZero(v);
  const flow = flowOf(margin);
  return {
    gap: flow.down,
    band: [margin.bottom, padding.bottom, border.bottom].some(set),
    top: [margin.top, padding.top, border.top].some(set),
    loose: /^(absolute|fixed)/.test(decl(body, "position") ?? ""),
    row: flow.row,
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
  for (const { selector, body } of held) {
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
export const upper = (n: ts.Node) => /^[A-Z]/.test(tag(n));
export const attrs = (n: ts.Node) => {
  const all: readonly ts.JsxAttributeLike[] =
    opening(n)?.attributes.properties ?? [];
  return all.filter(ts.isJsxAttribute);
};
export const attr = (n: ts.Node, name: string) =>
  attrs(n).find((a) => a.name.getText() === name);
// The object literals a spread can hand, through parentheses and conditions.
const spreadArms = (x: ts.Expression): ts.ObjectLiteralExpression[] => {
  if (ts.isParenthesizedExpression(x)) return spreadArms(x.expression);
  if (ts.isObjectLiteralExpression(x)) return [x];
  if (!ts.isConditionalExpression(x)) return [];
  const [a, b] = [spreadArms(x.whenTrue), spreadArms(x.whenFalse)];
  return a.length > 0 && b.length > 0 ? [...a, ...b] : [];
};
type Member = ts.JsxAttributeLike | ts.ObjectLiteralElementLike;
const memberKey = (m: Member) => {
  const key = m.name;
  if (!key || ts.isComputedPropertyName(key)) return undefined;
  return ts.isJsxNamespacedName(key) ? key.getText() : key.text;
};
const initOf = (m: Member) => {
  if (ts.isShorthandPropertyAssignment(m)) return m.name;
  if (ts.isPropertyAssignment(m)) return m.initializer;
  const init = ts.isJsxAttribute(m) ? m.initializer : undefined;
  return init && ts.isJsxExpression(init) ? init.expression : init;
};
type Reading = { values: ts.Expression[]; open: boolean } | "any";
// What one member hands `name`; `open` when it may leave the prop unset.
function memberReading(m: Member, name: string): Reading {
  if (ts.isJsxSpreadAttribute(m) || ts.isSpreadAssignment(m)) {
    const arms = spreadArms(m.expression).map((o) =>
      lastNamed(o.properties, name),
    );
    const read = arms.filter((r) => r !== "any");
    if (arms.length === 0 || read.length < arms.length) return "any";
    const values = read.flatMap((r) => r.values);
    return { values, open: read.some((r) => r.open) };
  }
  const key = memberKey(m);
  if (key === undefined) return "any";
  if (key !== name) return { values: [], open: true };
  const x = initOf(m);
  return x ? { values: [x], open: false } : "any";
}
// The last member naming `name` wins, as JSX and object spreads apply in
// source order.
function lastNamed(members: readonly Member[], name: string): Reading {
  const values: ts.Expression[] = [];
  for (const m of [...members].reverse()) {
    const read = memberReading(m, name);
    if (read === "any") return "any";
    values.push(...read.values);
    if (!read.open) return { values, open: false };
  }
  return { values, open: true };
}
/** Every value a call may hand prop `name`, `undefined` for a path that hands
 * none; "any" when a bare attribute or an unreadable spread hides the value. */
export function handedTo(
  e: ts.Node,
  name: string,
): (ts.Expression | undefined)[] | "any" {
  const read = lastNamed(opening(e)?.attributes.properties ?? [], name);
  if (read === "any") return "any";
  return read.open ? [...read.values, undefined] : read.values;
}
const below = new WeakMap<ts.Node, readonly ts.Node[]>();
const elementsBelow = new WeakMap<ts.Node, readonly ts.Node[]>();
export function descendants(root: ts.Node): readonly ts.Node[] {
  const known = below.get(root);
  if (known) return known;
  const out: ts.Node[] = [];
  const visit = (n: ts.Node) => {
    out.push(n);
    ts.forEachChild(n, visit);
  };
  ts.forEachChild(root, visit);
  below.set(root, out);
  return out;
}
export function elementsIn(root: ts.Node): readonly ts.Node[] {
  const known = elementsBelow.get(root) ?? descendants(root).filter(opening);
  elementsBelow.set(root, known);
  return known;
}
export const enclosing = (n: ts.Node) => {
  for (let p = n.parent; p; p = p.parent) if (ts.isJsxElement(p)) return p;
  return undefined;
};
/** The expressions an id-carrying attribute is written in, so two attributes
 * naming one id can be matched by their source text. */
export function idParts(a: ts.JsxAttribute | undefined): string[] {
  const init = a?.initializer;
  const e = init && ts.isJsxExpression(init) ? init.expression : init;
  if (e && ts.isTemplateExpression(e)) {
    return e.templateSpans.map((span) => span.expression.getText());
  }
  return e ? [e.getText()] : [];
}
/** Every class any branch of the element's `className` can carry. */
export const classes = (n: ts.Node) => [
  ...new Set(
    classVariants(n.getSourceFile(), attr(n, "className")?.initializer).flat(),
  ),
];
export const within = (n: ts.Node, outer: ts.Node) =>
  n.getSourceFile() === outer.getSourceFile() &&
  outer.pos <= n.pos &&
  n.end <= outer.end;

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

const frontendDir = join(srcDir, "..");
const manifest: { name: string; exports: Record<string, string> } = JSON.parse(
  readFileSync(join(frontendDir, "package.json"), "utf8"),
);
// An extension imports the core through the package's own exports map.
function moduleFor(from: string, spec: string): string | null {
  const own = `${manifest.name}/`;
  if (!spec.startsWith(own)) return resolveRelative(from, spec);
  const target = manifest.exports[`./${spec.slice(own.length)}`];
  const file = target && join(frontendDir, target);
  return file && existsSync(file) ? file : null;
}
// Where a component tag is defined, through imports, re-exports and renames.
const defined = new WeakMap<ts.SourceFile, Map<string, ts.Node | undefined>>();
export function definition(
  source: ts.SourceFile,
  name: string,
): ts.Node | undefined {
  const known = defined.get(source) ?? new Map<string, ts.Node | undefined>();
  defined.set(source, known);
  if (!known.has(name)) known.set(name, follow(source, name, 6));
  return known.get(name);
}
function follow(
  source: ts.SourceFile,
  name: string,
  hops: number,
): ts.Node | undefined {
  const local = declarations(source).get(name);
  const link = bindings(source).get(name);
  if (local || hops === 0 || !link) return local;
  const renamed = link.spec === "" && link.name !== name;
  if (link.spec === "" && !renamed) return undefined;
  const file = renamed
    ? source.fileName
    : moduleFor(source.fileName, link.spec);
  if (!file) return undefined;
  const from = renamed ? source : sourceFileAt(file);
  return follow(from, link.name, hops - 1);
}
export const definedName = (def: ts.Node) =>
  (ts.isFunctionDeclaration(def) || ts.isVariableDeclaration(def)) && def.name
    ? def.name.getText()
    : "";
export const keyOf = (def: ts.Node) =>
  `${def.getSourceFile().fileName}#${definedName(def)}`;
export const defOf = (n: ts.Node) =>
  upper(n) ? definition(n.getSourceFile(), tag(n)) : undefined;
/** The key of what `name` is in `design-system/<file>.tsx`. */
export function primitiveKey(file: string, name: string): string {
  const path = join(srcDir, "design-system", `${file}.tsx`);
  const def = definition(sourceFileAt(path), name);
  if (!def) throw new Error(`${path} no longer defines ${name}`);
  return keyOf(def);
}

export const fnOf = (def: ts.Node) =>
  ts.isVariableDeclaration(def) && def.initializer ? def.initializer : def;
const propsOf = (def: ts.Node) => {
  const fn = fnOf(def);
  return ts.isFunctionLike(fn) ? fn.parameters[0]?.name : undefined;
};
/** The props a component destructures, and `children` always. */
export function propNames(def: ts.Node): Set<string> {
  const first = propsOf(def);
  const names = new Set(["children"]);
  if (first && ts.isObjectBindingPattern(first)) {
    for (const b of first.elements) names.add(b.name.getText());
  }
  return names;
}
// The names a component is handed: its destructured props, or `props.x`.
export function givenTo(def: ts.Node) {
  const first = propsOf(def);
  const names = propNames(def);
  const props = first && ts.isIdentifier(first) ? first.text : undefined;
  return (e: ts.Node, name?: string) =>
    (ts.isIdentifier(e) &&
      names.has(e.text) &&
      (name === undefined || e.text === name)) ||
    (ts.isPropertyAccessExpression(e) &&
      e.expression.getText() === props &&
      (name === undefined || e.name.text === name));
}
/** The literal a component's destructured prop defaults to. */
export function presetOf(def: ts.Node, name: string): string | undefined {
  const first = propsOf(def);
  if (!first || !ts.isObjectBindingPattern(first)) return undefined;
  const b = first.elements.find((e) => e.name.getText() === name);
  return b?.initializer && ts.isStringLiteral(b.initializer)
    ? b.initializer.text
    : undefined;
}

// The string literals a type admits: a union of them, a local alias of one,
// or `(typeof LIST)[number]` over a local `as const` list.
function literalTypes(type: ts.TypeNode | undefined): string[] | undefined {
  if (!type) return undefined;
  if (ts.isParenthesizedTypeNode(type)) return literalTypes(type.type);
  if (ts.isLiteralTypeNode(type) && ts.isStringLiteral(type.literal)) {
    return [type.literal.text];
  }
  if (ts.isUnionTypeNode(type)) {
    const each = type.types.map(literalTypes);
    return each.every((t) => t) ? each.flatMap((t) => t ?? []) : undefined;
  }
  if (ts.isTypeReferenceNode(type)) {
    const alias = type
      .getSourceFile()
      .statements.find(
        (s): s is ts.TypeAliasDeclaration =>
          ts.isTypeAliasDeclaration(s) &&
          s.name.text === type.typeName.getText(),
      );
    return alias && literalTypes(alias.type);
  }
  return ts.isIndexedAccessTypeNode(type) ? listed(type) : undefined;
}
function listed(type: ts.IndexedAccessTypeNode): string[] | undefined {
  const list =
    type.indexType.getText() === "number"
      ? /^\(?typeof (\w+)\)?$/.exec(type.objectType.getText())?.[1]
      : undefined;
  const held = list ? declarations(type.getSourceFile()).get(list) : undefined;
  const init =
    held && ts.isVariableDeclaration(held) ? held.initializer : undefined;
  const array = init && ts.isAsExpression(init) ? init.expression : init;
  if (!array || !ts.isArrayLiteralExpression(array)) return undefined;
  const items = array.elements.filter(ts.isStringLiteral).map((e) => e.text);
  return items.length === array.elements.length ? items : undefined;
}
/** The values a component's prop is typed to take, where its type spells them. */
export function domainOf(def: ts.Node, name: string): string[] | undefined {
  const fn = fnOf(def);
  let type = ts.isFunctionLike(fn) ? fn.parameters[0]?.type : undefined;
  while (type && ts.isTypeReferenceNode(type) && type.typeArguments?.length) {
    type = type.typeArguments[0];
  }
  const member =
    type && ts.isTypeLiteralNode(type)
      ? type.members.find(
          (m): m is ts.PropertySignature =>
            ts.isPropertySignature(m) && m.name.getText() === name,
        )
      : undefined;
  return literalTypes(member?.type);
}

// The JSX an expression evaluates to, through parentheses and conditions.
export function jsxOf(e: ts.Node | undefined): ts.Node[] {
  if (!e) return [];
  if (ts.isParenthesizedExpression(e)) return jsxOf(e.expression);
  if (opening(e) || ts.isJsxFragment(e)) return [e];
  if (ts.isConditionalExpression(e)) {
    return [...jsxOf(e.whenTrue), ...jsxOf(e.whenFalse)];
  }
  return ts.isBinaryExpression(e) ? jsxOf(e.right) : [];
}
export function returnedBy(fn: ts.Node): ts.Node[] {
  if (!ts.isFunctionLike(fn)) return [];
  if (ts.isArrowFunction(fn) && !ts.isBlock(fn.body)) return jsxOf(fn.body);
  return descendants(fn)
    .filter(ts.isReturnStatement)
    .filter((r) => ts.findAncestor(r.parent, ts.isFunctionLike) === fn)
    .flatMap((r) => jsxOf(r.expression));
}

// The elements a component renders at its root. JSX a named local function
// builds lands where that function is used, at the root only if a use is.
export function rootsOf(def: ts.Node): ts.Node[] {
  const body = fnOf(def);
  const nameOf = (fn: ts.Node) => {
    if (ts.isFunctionDeclaration(fn)) return fn.name;
    const bound = fn.parent;
    return ts.isVariableDeclaration(bound) && ts.isIdentifier(bound.name)
      ? bound.name
      : undefined;
  };
  const atRoot = (n: ts.Node, hops: number): boolean => {
    const parent = enclosing(n);
    if (parent && within(parent, def)) return false;
    const fn = ts.findAncestor(n.parent, ts.isFunctionLike);
    const name = fn && fn !== body && within(fn, def) && nameOf(fn);
    if (!fn || !name || hops === 0) return true;
    return descendants(def).some(
      (use) =>
        ts.isIdentifier(use) &&
        use.text === name.text &&
        use !== name &&
        !within(use, fn) &&
        atRoot(use, hops - 1),
    );
  };
  return elementsIn(def).filter((e) => atRoot(e, 4));
}

/** The string values an expression can take, `undefined` for unset; none
 * when it is not written in literals. */
export function literalsIn(
  x: ts.Expression,
): (string | undefined)[] | undefined {
  if (ts.isStringLiteral(x) || ts.isNoSubstitutionTemplateLiteral(x)) {
    return [x.text];
  }
  if (x.kind === ts.SyntaxKind.NullKeyword || x.getText() === "undefined") {
    return [undefined];
  }
  if (ts.isParenthesizedExpression(x)) return literalsIn(x.expression);
  if (!ts.isConditionalExpression(x)) return undefined;
  const [a, b] = [literalsIn(x.whenTrue), literalsIn(x.whenFalse)];
  return a && b ? [...a, ...b] : undefined;
}
// What an attribute hands a component to render: JSX, or a function's JSX.
export function handedJsx(a: ts.JsxAttribute): ts.Node[] {
  const init = a.initializer;
  const e = init && ts.isJsxExpression(init) ? init.expression : undefined;
  if (!e) return [];
  const fn = ts.isArrowFunction(e) || ts.isFunctionExpression(e);
  return fn ? returnedBy(e) : jsxOf(e);
}
// What a JSX child renders: `{open && body(n)}` renders `body`.
export function rendered(e: ts.Expression): ts.Expression[] {
  if (ts.isParenthesizedExpression(e)) return rendered(e.expression);
  if (ts.isConditionalExpression(e)) {
    return [...rendered(e.whenTrue), ...rendered(e.whenFalse)];
  }
  if (ts.isCallExpression(e)) return [e.expression];
  if (!ts.isBinaryExpression(e)) return [e];
  return e.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken
    ? rendered(e.right)
    : [...rendered(e.left), ...rendered(e.right)];
}
export const callOf = (a: ts.JsxAttribute) => {
  const on = a.parent.parent;
  return ts.isJsxOpeningElement(on) ? on.parent : on;
};

/** The element a component renders the `{children}` it is handed into, where
 * the classes it draws around a caller's content land. */
export function childrenSlot(component: ts.Node): ts.Node | undefined {
  const def = defOf(component);
  const given = def && givenTo(def);
  const slot =
    def &&
    descendants(def).find(
      (x) =>
        ts.isJsxExpression(x) &&
        !!x.expression &&
        !!given?.(x.expression, "children"),
    );
  return slot && enclosing(slot);
}
/** The components in `def` that enclose the `{children}` it is handed. */
export function childrenHosts(def: ts.Node): ts.Node[] {
  const given = givenTo(def);
  const around = new Set<ts.Node>();
  for (const x of descendants(def)) {
    const handed =
      ts.isJsxExpression(x) &&
      !!x.expression &&
      given(x.expression, "children");
    for (let p = handed ? enclosing(x) : undefined; p; p = enclosing(p)) {
      if (!within(p, def)) break;
      if (upper(p)) around.add(p);
    }
  }
  return [...around];
}
export const isDialogTag = (
  file: ts.SourceFile,
  name: string,
  keys: ReadonlySet<string>,
) => {
  const def = definition(file, name);
  return !!def && keys.has(keyOf(def));
};
// A component that renders a dialog around its children is a dialog, to a
// fixed point from `Modal`.
function dialogKeys(
  files: readonly ts.SourceFile[],
  seed: Iterable<string>,
): Set<string> {
  const keys = new Set(seed);
  const candidates = files.flatMap((file) =>
    [...declarations(file)]
      .filter(([name]) => /^[A-Z]/.test(name))
      .map(([, def]) => ({ file, def, around: childrenHosts(def) }))
      .filter((c) => c.around.length > 0),
  );
  for (let grew = true; grew; ) {
    grew = false;
    for (const { file, def, around } of candidates) {
      if (keys.has(keyOf(def))) continue;
      if (around.some((e) => isDialogTag(file, tag(e), keys))) {
        keys.add(keyOf(def));
        grew = true;
      }
    }
  }
  return keys;
}
export type DialogSet = { modal: string; keys: Set<string> };
let treeDialogs: DialogSet | undefined;
/** `Modal`'s key and every dialog key: over the tree, cached, or over
 * `files` grown from the keys already known in `from`. */
export function dialogSet(
  files?: readonly ts.SourceFile[],
  from?: DialogSet,
): DialogSet {
  if (!files && treeDialogs) return treeDialogs;
  const modal = primitiveKey("modal", "Modal");
  const read = files ?? markupFiles().map((f) => sourceFileAt(f));
  const set = { modal, keys: dialogKeys(read, from?.keys ?? [modal]) };
  if (!files) treeDialogs = set;
  return set;
}

const DECLARATION =
  /^(?:export\s+(?:default\s+)?)?(?:function\s+([A-Z]\w*)|const\s+([A-Z]\w*)\s*(?::[^=]*)?=)/gm;
function declaredIn(text: string) {
  const starts = [...text.matchAll(DECLARATION)];
  return starts.map((m, i) => ({
    name: m[1] ?? m[2],
    text: text.slice(m.index, starts[i + 1]?.index ?? text.length),
  }));
}
const wrapsChildren = (text: string, name: string) =>
  [...text.matchAll(new RegExp(`<${name}\\b[\\s\\S]*?</${name}>`, "g"))].some(
    (m) => /\{\s*(?:props\.)?children\s*\}/.test(m[0]),
  );
const aliases = (text: string, names: Set<string>) =>
  [...text.matchAll(/\b(\w+) as (\w+)\b/g)]
    .filter((m) => names.has(m[1]))
    .map((m) => m[2]);
// A name a file can spell as a tag: one it declares, imports or renames to.
const spells = (text: string, name: string) =>
  new RegExp(
    `\\b(?:function|const)\\s+${name}\\b|import\\s*\\{[^}]*\\b${name}\\b[^}]*\\}|\\bas\\s+${name}\\b`,
  ).test(text);
// Not after a name: `useState<Ask>` is a type argument, not a tag.
const tagsOf = (text: string, names: string[]) =>
  names.length === 0
    ? 0
    : (text.match(new RegExp(`(?<![\\w$.])<(?:${names.join("|")})\\b`, "g"))
        ?.length ?? 0);
// A second count of the dialogs, read from text without the syntax tree, so a
// walk that misses one disagrees with it.
export function textCensus(texts: readonly string[]) {
  const files = texts.map((t) => {
    const text = t
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .replace(/(^|\s)\/\/.*$/gm, "$1");
    return { text, seen: new Map<string, boolean>() };
  });
  const visible = (file: (typeof files)[number], name: string) => {
    const known = file.seen.get(name) ?? spells(file.text, name);
    file.seen.set(name, known);
    return known;
  };
  const decls = files.flatMap((file) =>
    declaredIn(file.text).map((d) => ({ ...d, file })),
  );
  const wraps = (d: (typeof decls)[number], names: Set<string>) =>
    [...names].some((n) => visible(d.file, n) && wrapsChildren(d.text, n));
  const grow = (names: Set<string>, wrappers: boolean) => {
    for (let size = 0; size !== names.size; ) {
      size = names.size;
      const renamed = files.flatMap((f) => aliases(f.text, names));
      const wrapping = wrappers ? decls.filter((d) => wraps(d, names)) : [];
      for (const n of [...renamed, ...wrapping.map((d) => d.name)]) {
        names.add(n);
      }
    }
    return names;
  };
  const modal = grow(new Set(["Modal"]), false);
  const names = grow(new Set(modal), true);
  const count = (set: Set<string>) =>
    files.reduce(
      (n, f) =>
        n +
        tagsOf(
          f.text,
          [...set].filter((s) => visible(f, s)),
        ),
      0,
    );
  const roles = files.reduce(
    (n, f) =>
      n + (f.text.match(/\srole=(?:"dialog"|\{"dialog"\})/g)?.length ?? 0),
    0,
  );
  return { modals: count(modal), dialogs: count(names) + roles, names };
}

type Args = ReadonlyMap<string, string | undefined>;
const MODAL_FILE = join(srcDir, "design-system", "modal.tsx");
const unread = (n: ts.Node, what: string) =>
  new Error(
    `${n.getSourceFile().fileName}: modalClass ${what} \`${n.getText()}\`, which the dialog box reader does not evaluate; extend evaluate() in dialoglayout.ts`,
  );
const COMPARE = new Map([
  [ts.SyntaxKind.EqualsEqualsEqualsToken, true],
  [ts.SyntaxKind.ExclamationEqualsEqualsToken, false],
]);
function evaluate(e: ts.Expression, args: Args): string | boolean | undefined {
  if (ts.isParenthesizedExpression(e)) return evaluate(e.expression, args);
  if (ts.isStringLiteral(e)) return e.text;
  if (ts.isIdentifier(e)) {
    if (!args.has(e.text)) throw unread(e, "reads");
    return args.get(e.text);
  }
  if (ts.isConditionalExpression(e)) {
    const pick = evaluate(e.condition, args) ? e.whenTrue : e.whenFalse;
    return evaluate(pick, args);
  }
  const equal = ts.isBinaryExpression(e)
    ? COMPARE.get(e.operatorToken.kind)
    : undefined;
  if (!ts.isBinaryExpression(e) || equal === undefined) {
    throw unread(e, "is written in");
  }
  return (evaluate(e.left, args) === evaluate(e.right, args)) === equal;
}
const statementsOf = (s: ts.Statement | undefined) => {
  if (!s) return [];
  return ts.isBlock(s) ? s.statements : [s];
};
function execute(
  statements: readonly ts.Statement[],
  args: Args,
): string | undefined {
  for (const s of statements) {
    if (ts.isReturnStatement(s) && s.expression) {
      const out = evaluate(s.expression, args);
      if (typeof out !== "string") throw unread(s, "returns a non-string from");
      return out;
    }
    if (!ts.isIfStatement(s)) throw unread(s, "runs");
    const taken = evaluate(s.expression, args);
    const out = execute(
      statementsOf(taken ? s.thenStatement : s.elseStatement),
      args,
    );
    if (out !== undefined) return out;
  }
  return undefined;
}
// Each of modalClass's parameters, the Modal prop the call site hands it, the
// values its type admits (`undefined` for an optional prop) and its default.
type Param = {
  name: string;
  prop: string;
  domain: (string | undefined)[];
  preset?: string;
};
const literalsOf = (type: ts.TypeNode | undefined) => {
  const members = type && ts.isUnionTypeNode(type) ? type.types : [type];
  const each = members.map((m) =>
    m?.kind === ts.SyntaxKind.UndefinedKeyword ? [undefined] : literalTypes(m),
  );
  return each.every((m) => m) ? each.flatMap((m) => m ?? []) : [];
};
type Shape = { fn: ts.FunctionDeclaration; params: Param[] };
const shapes = new Map<string, Shape>();
function shapeOfModal(path: string): Shape {
  const known = shapes.get(path);
  if (known) return known;
  const file = sourceFileAt(path);
  const fn = declarations(file).get("modalClass");
  const modal = declarations(file).get("Modal");
  const call =
    modal &&
    descendants(modal).find(
      (n): n is ts.CallExpression =>
        ts.isCallExpression(n) && n.expression.getText() === "modalClass",
    );
  if (!fn || !ts.isFunctionDeclaration(fn) || !fn.body || !modal || !call) {
    throw new Error(
      `${path}: Modal no longer draws its box with className={modalClass(…)}; teach shapeOfModal() in dialoglayout.ts the new shape`,
    );
  }
  const params = fn.parameters.map((p, i): Param => {
    const arg = call.arguments[i];
    const domain = literalsOf(p.type);
    if (!arg || !ts.isIdentifier(arg) || domain.length === 0) {
      throw unread(
        p,
        "takes, other than a Modal prop typed as string literals,",
      );
    }
    const preset = presetOf(modal, arg.text);
    return { name: p.name.getText(), prop: arg.text, domain, preset };
  });
  shapes.set(path, { fn, params });
  return { fn, params };
}
/** The Modal props whose value decides the box classes. */
export const boxProps = (path = MODAL_FILE) =>
  shapeOfModal(path).params.map((p) => p.prop);
/** A prop's possible values: listed, `undefined` for unset, or any it admits. */
export type Values = readonly (string | undefined)[] | "any";
/** The box classes `Modal` draws for each combination of the values given,
 * read off `modalClass` and the props' defaults in modal.tsx. */
export function modalBoxes(
  given: ReadonlyMap<string, Values>,
  path = MODAL_FILE,
): string[][] {
  const { fn, params } = shapeOfModal(path);
  const choices = params.map((p) => {
    const v = given.get(p.prop) ?? [undefined];
    return (v === "any" ? p.domain : v).map((x) => x ?? p.preset);
  });
  const combos = choices.reduce<(string | undefined)[][]>(
    (all, values) => all.flatMap((a) => values.map((v) => [...a, v])),
    [[]],
  );
  const boxes = combos.map((combo) => {
    const args = new Map(params.map((p, i) => [p.name, combo[i]]));
    const out = execute(fn.body?.statements ?? [], args);
    if (!out) throw unread(fn, `returns no class for ${combo.join("/")} in`);
    return out;
  });
  // Combinations that draw the same box are judged once.
  return [...new Set(boxes)].map((out) => out.split(/\s+/));
}

const CAP = 64;
export const whereOf = (n: ts.Node) => {
  const file = n.getSourceFile();
  const { line } = file.getLineAndCharacterOfPosition(n.getStart());
  return `${relative(srcDir, file.fileName)}:${line + 1}`;
};
export function capped<T>(all: T[], at: ts.Node): T[] {
  if (all.length <= CAP) return all;
  throw new Error(
    `${whereOf(at)}: more than ${CAP} class combinations to judge; name the branches so fewer combine`,
  );
}
export const cross = (a: string[][], b: string[][], at: ts.Node) =>
  capped(
    a.flatMap((x) => b.map((y) => [...x, ...y])),
    at,
  );
const escaped = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
// A pattern class (`ds-gap-*`) reads as each class it can be, one at a time.
export function expanded(
  owners: Owners,
  cls: string[],
  at: ts.Node,
): string[][] {
  const known = [...owners.placed.keys()];
  return cls.reduce<string[][]>(
    (all, c) => {
      if (!c.includes("*")) return all.map((a) => [...a, c]);
      const shape = new RegExp(
        `^${c.split("*").map(escaped).join("[\\w-]*")}$`,
      );
      const hits = known.filter((k) => shape.test(k)).map((k) => [k]);
      return hits.length > 0 ? cross(all, hits, at) : all;
    },
    [[]],
  );
}
