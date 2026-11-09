// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every screen that renders inside the railed shell insets its content from
// the scroller's edges.
//
// The gutter is `--pageGutter`, and `.wrap` (app/shell.css) is its one
// spelling: the inset the page head above the screen already takes. The shell
// does not apply `.wrap`; every screen writes it on its own root.
//
// The lists come from the files that own them: `SCREENS` in lib/router.tsx,
// the `SCREEN_VIEWS` dispatch in App.views.tsx, and `RAIL_LESS_SCREENS` in
// app/nav.ts. The first two must agree, so no destination goes unjudged. A
// render in jsdom would need a harness per screen, a hand list in disguise.
//
// What this cannot see:
// - Amount. A screen padded by 2px passes; only `.wrap` is the product's number.
// - The cascade. A gutter is `.wrap` or a bare class rule with padding. A
//   compound selector, an inline style or a parent's padding reads as absent.
// - A className built at runtime, which reads as unreadable.
// - A screen from a generated registry, named in NOT_OURS_TO_INSET.
// - A negative margin inside that gives the inset back.
// - A gutter inside a wrapper's children. The walk follows a wrapper to its
//   own root only, since a `.wrap` inside a query gate leaves its pending and
//   refusal states flush. Lift such a gutter to the screen's own root.

import { join } from "node:path";
import ts from "typescript";
import { resolveRelative } from "../../scripts/lib/source-tree";
import { stable } from "../../src/format/collate";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  srcDir,
} from "../tree";

export const ROUTER = join(srcDir, "lib/router.tsx");
export const NAV = join(srcDir, "app/nav.ts");
export const APP = join(srcDir, "App.tsx");
export const APP_SCREENS = join(srcDir, "App.screens.tsx");
export const APP_VIEWS = join(srcDir, "App.views.tsx");

/**
 * The screens whose surface this build does not author, so there is no root of
 * ours to inset. Each is checked against `SCREENS`, so a retired destination
 * fails rather than sitting on as a stale excuse.
 */
const NOT_OURS_TO_INSET: ReadonlyMap<string, string> = new Map([
  [
    "ext",
    "a composed unit's screen arrives from the generated @composition/screens registry; the unit lays its own surface out, and the two branches this build does draw for it are inset",
  ],
  [
    "x",
    "a fork's screen arrives from app/custom.ts's registry, which is empty upstream; the fallback this build draws for it is inset",
  ],
]);

/** The tree a walk reads modules from, and what it has learnt so far. */
export type Walk = Readonly<{
  at: (path: string) => ts.SourceFile | undefined;
  exists: (path: string) => boolean;
  insetting: ReadonlySet<string>;
  bindings: Map<ts.SourceFile, ReadonlyMap<string, Binding>>;
}>;

export function walkOver(
  tree: CensusTree,
  insetting: ReadonlySet<string>,
): Walk {
  return {
    at: (path) => tree.at(path)?.source,
    exists: (path) => tree.at(path)?.source !== undefined,
    insetting,
    bindings: new Map(),
  };
}

function declarationNamed(
  file: ts.SourceFile,
  name: string,
): ts.VariableDeclaration | undefined {
  let found: ts.VariableDeclaration | undefined;
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === name
    ) {
      found = node;
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
  return found;
}

/** Every string literal under a node, which is how a closed list is read. */
function stringsIn(node: ts.Node | undefined): string[] {
  if (!node) {
    return [];
  }
  const out: string[] = [];
  const visit = (child: ts.Node) => {
    if (ts.isStringLiteralLike(child)) {
      out.push(child.text);
    }
    ts.forEachChild(child, visit);
  };
  visit(node);
  return out;
}

type Binding = { file: string; exported: string };

/**
 * Which module each name in a file comes from. The sources are static imports,
 * `export … from` re-exports, and one lazy form. Every routed screen in
 * App.screens.tsx is declared with that form: `lazy(routed(() =>
 * import("./x").then((m) => ({ default: m.Y }))))`.
 */
function bindingsOf(
  walk: Walk,
  file: ts.SourceFile,
): ReadonlyMap<string, Binding> {
  const hit = walk.bindings.get(file);
  if (hit) {
    return hit;
  }
  const out = new Map<string, Binding>();
  for (const statement of file.statements) {
    if (ts.isImportDeclaration(statement)) {
      readImport(walk, file, statement, out);
    } else if (ts.isExportDeclaration(statement)) {
      readReExport(walk, file, statement, out);
    } else if (ts.isVariableStatement(statement)) {
      readLazyImport(walk, file, statement, out);
    }
  }
  walk.bindings.set(file, out);
  return out;
}

function readImport(
  walk: Walk,
  file: ts.SourceFile,
  statement: ts.ImportDeclaration,
  out: Map<string, Binding>,
): void {
  if (
    !ts.isStringLiteral(statement.moduleSpecifier) ||
    !statement.importClause
  ) {
    return;
  }
  const target = resolveRelative(
    file.fileName,
    statement.moduleSpecifier.text,
    walk.exists,
  );
  if (!target) {
    return;
  }
  const { name, namedBindings } = statement.importClause;
  if (name) {
    out.set(name.text, { file: target, exported: "default" });
  }
  if (namedBindings && ts.isNamedImports(namedBindings)) {
    for (const element of namedBindings.elements) {
      out.set(element.name.text, {
        file: target,
        exported: (element.propertyName ?? element.name).text,
      });
    }
  }
}

/** `export { LeadsScreen } from "./leads.list"`: a screen's file can be a door. */
function readReExport(
  walk: Walk,
  file: ts.SourceFile,
  statement: ts.ExportDeclaration,
  out: Map<string, Binding>,
): void {
  const specifier = statement.moduleSpecifier;
  if (
    !specifier ||
    !ts.isStringLiteral(specifier) ||
    !statement.exportClause ||
    !ts.isNamedExports(statement.exportClause)
  ) {
    return;
  }
  const target = resolveRelative(file.fileName, specifier.text, walk.exists);
  if (!target) {
    return;
  }
  for (const element of statement.exportClause.elements) {
    out.set(element.name.text, {
      file: target,
      exported: (element.propertyName ?? element.name).text,
    });
  }
}

function readLazyImport(
  walk: Walk,
  file: ts.SourceFile,
  statement: ts.VariableStatement,
  out: Map<string, Binding>,
): void {
  for (const declaration of statement.declarationList.declarations) {
    if (!ts.isIdentifier(declaration.name) || !declaration.initializer) {
      continue;
    }
    const specifier = dynamicImportSpecifier(declaration.initializer);
    const target = specifier
      ? resolveRelative(file.fileName, specifier, walk.exists)
      : null;
    if (!target) {
      continue;
    }
    out.set(declaration.name.text, {
      file: target,
      exported: lazyExportName(declaration.initializer) ?? "default",
    });
  }
}

function dynamicImportSpecifier(node: ts.Node): string | undefined {
  let specifier: string | undefined;
  const visit = (child: ts.Node) => {
    if (
      ts.isCallExpression(child) &&
      child.expression.kind === ts.SyntaxKind.ImportKeyword
    ) {
      const [argument] = child.arguments;
      if (argument && ts.isStringLiteral(argument)) {
        specifier = argument.text;
      }
    }
    ts.forEachChild(child, visit);
  };
  visit(node);
  return specifier;
}

/** The `m.Screen` in `.then((m) => ({ default: m.Screen }))`. */
function lazyExportName(node: ts.Node): string | undefined {
  let name: string | undefined;
  const visit = (child: ts.Node) => {
    if (
      ts.isPropertyAssignment(child) &&
      ts.isIdentifier(child.name) &&
      child.name.text === "default" &&
      ts.isPropertyAccessExpression(child.initializer)
    ) {
      name = child.initializer.name.text;
    }
    ts.forEachChild(child, visit);
  };
  visit(node);
  return name;
}

/** The body of a component declared as a function or as an arrow constant. */
export function bodyOfComponent(
  file: ts.SourceFile,
  name: string,
): ts.Node | undefined {
  let body: ts.Node | undefined;
  const visit = (node: ts.Node) => {
    if (
      ts.isFunctionDeclaration(node) &&
      node.name?.text === name &&
      node.body
    ) {
      body = node.body;
    }
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === name &&
      node.initializer &&
      (ts.isArrowFunction(node.initializer) ||
        ts.isFunctionExpression(node.initializer))
    ) {
      body = node.initializer.body;
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
  return body;
}

/**
 * The file that declares a component, following as many doors as it takes.
 * App.screens.tsx's `lazy` constant is one door. A screen file that re-exports
 * its list from a sibling (`export { LeadsScreen } from "./leads.list"`) is
 * another.
 */
function componentSource(
  walk: Walk,
  file: ts.SourceFile,
  name: string,
  hops = 0,
): { file: ts.SourceFile; name: string } | undefined {
  if (bodyOfComponent(file, name)) {
    return { file, name };
  }
  const binding = bindingsOf(walk, file).get(name);
  // The cap stops a re-export cycle; no chain in this tree is more than two
  // doors long.
  const next = binding?.file.startsWith(srcDir)
    ? walk.at(binding.file)
    : undefined;
  if (!binding || !next || hops >= 8) {
    return undefined;
  }
  return componentSource(walk, next, binding.exported, hops + 1);
}

/** `export const EXTENSION_SCREEN = "ext"`, followed through the import. */
function constantString(
  walk: Walk,
  file: ts.SourceFile,
  name: string,
): string | undefined {
  const local = declarationNamed(file, name);
  if (local?.initializer && ts.isStringLiteralLike(local.initializer)) {
    return local.initializer.text;
  }
  const binding = bindingsOf(walk, file).get(name);
  const next = binding ? walk.at(binding.file) : undefined;
  return binding && next
    ? constantString(walk, next, binding.exported)
    : undefined;
}

type Root =
  | { kind: "element"; node: ts.JsxOpeningLikeElement }
  | { kind: "nothing" }
  | { kind: "unreadable"; text: string };

export function rootsOfBody(body: ts.Node): Root[] {
  if (!ts.isBlock(body)) {
    return ts.isExpression(body) ? rootsOfExpression(body) : [];
  }
  const returned: ts.Expression[] = [];
  const visit = (node: ts.Node) => {
    // A nested function's returns are its own, not this component's.
    if (
      ts.isFunctionDeclaration(node) ||
      ts.isFunctionExpression(node) ||
      ts.isArrowFunction(node)
    ) {
      return;
    }
    if (ts.isReturnStatement(node) && node.expression) {
      returned.push(node.expression);
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(body, visit);
  return returned.flatMap(rootsOfExpression);
}

function rootsOfExpression(expression: ts.Expression): Root[] {
  if (ts.isParenthesizedExpression(expression)) {
    return rootsOfExpression(expression.expression);
  }
  // A render prop, `<QueryGate>{(session) => <Workbench …/>}</QueryGate>`, is a
  // child that draws, so what it returns is what stands there.
  if (ts.isArrowFunction(expression) || ts.isFunctionExpression(expression)) {
    return rootsOfBody(expression.body);
  }
  if (ts.isConditionalExpression(expression)) {
    return [
      ...rootsOfExpression(expression.whenTrue),
      ...rootsOfExpression(expression.whenFalse),
    ];
  }
  if (
    ts.isBinaryExpression(expression) &&
    (expression.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken ||
      expression.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken)
  ) {
    return rootsOfExpression(expression.right);
  }
  if (ts.isJsxElement(expression)) {
    return [{ kind: "element", node: expression.openingElement }];
  }
  if (ts.isJsxSelfClosingElement(expression)) {
    return [{ kind: "element", node: expression }];
  }
  if (ts.isJsxFragment(expression)) {
    return expression.children.flatMap(rootsOfChild);
  }
  // A branch that draws nothing has nothing to inset.
  if (
    expression.kind === ts.SyntaxKind.NullKeyword ||
    (ts.isIdentifier(expression) && expression.text === "undefined")
  ) {
    return [{ kind: "nothing" }];
  }
  return [{ kind: "unreadable", text: oneLine(expression) }];
}

function rootsOfChild(child: ts.JsxChild): Root[] {
  if (ts.isJsxElement(child)) {
    return [{ kind: "element", node: child.openingElement }];
  }
  if (ts.isJsxSelfClosingElement(child)) {
    return [{ kind: "element", node: child }];
  }
  if (ts.isJsxExpression(child) && child.expression) {
    return rootsOfExpression(child.expression);
  }
  return [];
}

function oneLine(node: ts.Node): string {
  return node.getText().replace(/\s+/g, " ").slice(0, 72);
}

const GUTTER_CLASS = "wrap";

/**
 * The classes on an element, but only where one value is the whole answer.
 *
 * A computed `className` is unreadable and says so. Pooling its literals would
 * read `className={folded ? "wrap a" : "b"}` as inset while the other branch
 * reaches the reader flush. A screen spells its root's classes as a literal or
 * takes a named exemption.
 */
function classesOn(element: ts.JsxOpeningLikeElement): string[] | undefined {
  for (const attribute of element.attributes.properties) {
    if (
      ts.isJsxAttribute(attribute) &&
      ts.isIdentifier(attribute.name) &&
      attribute.name.text === "className"
    ) {
      const literal = soleLiteral(attribute.initializer);
      return literal?.split(/\s+/).filter(Boolean);
    }
  }
  return [];
}

/** The one string an initializer is, or undefined when it is computed. */
function soleLiteral(node: ts.Node | undefined): string | undefined {
  if (!node) {
    return undefined;
  }
  if (ts.isStringLiteralLike(node)) {
    return node.text;
  }
  if (ts.isJsxExpression(node)) {
    return node.expression ? soleLiteral(node.expression) : undefined;
  }
  return undefined;
}

/**
 * Bare class selectors whose rule declares a non-zero horizontal padding: the
 * public pages outside the shell's column that spell their own inset
 * (`.pref-page`). A compound selector describes a class in a context, so only
 * a bare one counts.
 */
export function selfInsettingClasses(
  sheets: readonly CensusFile[],
): ReadonlySet<string> {
  const out = new Set<string>();
  for (const sheet of sheets) {
    // Comments out first: they hold prose about padding, and braces.
    const text = sheet.text.replace(/\/\*[\s\S]*?\*\//g, "");
    for (const [, selector, block] of text.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      const name = /^\s*\.([A-Za-z0-9_-]+)\s*$/.exec(selector)?.[1];
      if (name && declaresHorizontalPadding(block)) {
        out.add(name);
      }
    }
  }
  return out;
}

function declaresHorizontalPadding(block: string): boolean {
  // The block ones are absent from the alternation on purpose: `padding-top`
  // insets nothing horizontally, and reading it as a gutter would pass a screen
  // whose content still touches both edges.
  for (const [, property, value] of block.matchAll(
    /(padding(?:-inline(?:-start|-end)?|-left|-right)?)\s*:\s*([^;]+)/g,
  )) {
    if (!isZero(horizontalValue(property, value.trim()))) {
      return true;
    }
  }
  return false;
}

/** The horizontal half of a padding declaration, as written. */
function horizontalValue(property: string, value: string): string | undefined {
  // Split on the spaces between values, never on one inside `var(--a, 10px)`.
  const parts = value.split(/\s+(?![^(]*\))/);
  // Only the shorthand puts the horizontal half second; every longhand that
  // reaches here already names a horizontal edge.
  return property === "padding" && parts.length > 1 ? parts[1] : parts[0];
}

function isZero(value: string | undefined): boolean {
  return value === undefined || /^0[a-z%]*$/i.test(value);
}

type Verdict = { inset: boolean; trail: string };

export function insetVerdict(
  file: ts.SourceFile,
  roots: readonly Root[],
  walk: Walk,
  seen: ReadonlySet<string>,
): Verdict {
  const trail: string[] = [];
  for (const root of roots) {
    if (root.kind === "nothing") {
      continue;
    }
    if (root.kind === "unreadable") {
      return {
        inset: false,
        trail: `a root this walk cannot read: ${root.text}`,
      };
    }
    const verdict = elementVerdict(file, root.node, walk, seen);
    if (!verdict.inset) {
      return verdict;
    }
    trail.push(verdict.trail);
  }
  return { inset: true, trail: trail.join(" · ") };
}

function elementVerdict(
  file: ts.SourceFile,
  element: ts.JsxOpeningLikeElement,
  walk: Walk,
  seen: ReadonlySet<string>,
): Verdict {
  const tag = element.tagName.getText();
  if (reactSuspense(file, tag) && ts.isJsxElement(element.parent)) {
    if (element.attributes.properties.some(ts.isJsxSpreadAttribute)) {
      return {
        inset: false,
        trail: "Suspense fallback comes from unreadable spread props",
      };
    }
    const fallback = element.attributes.properties.find(
      (attribute) =>
        ts.isJsxAttribute(attribute) && attribute.name.getText() === "fallback",
    );
    const expression =
      fallback &&
      ts.isJsxAttribute(fallback) &&
      fallback.initializer &&
      ts.isJsxExpression(fallback.initializer)
        ? fallback.initializer.expression
        : fallback && ts.isJsxAttribute(fallback)
          ? fallback.initializer
          : undefined;
    const roots = element.parent.children.flatMap(rootsOfChild);
    return insetVerdict(
      file,
      [...roots, ...(expression ? rootsOfExpression(expression) : [])],
      walk,
      seen,
    );
  }
  if (/^[a-z]/.test(tag)) {
    const classes = classesOn(element);
    if (classes === undefined) {
      return {
        inset: false,
        trail: `root <${tag}> computes its className, so which classes it draws cannot be read here`,
      };
    }
    if (classes.includes(GUTTER_CLASS)) {
      return { inset: true, trail: `<${tag} class="${classes.join(" ")}">` };
    }
    if (classes.some((name) => walk.insetting.has(name))) {
      return { inset: true, trail: `<${tag}> insets itself in CSS` };
    }
    return {
      inset: false,
      trail: `root <${tag} class="${classes.join(" ")}"> takes no page gutter`,
    };
  }
  const own = componentVerdict(file, tag, walk, seen);
  if (own?.inset) {
    return own;
  }
  // A wrapper that insets only what it wraps is not an inset screen. Its
  // pending body or refusal would reach the reader flush. So the walk does not
  // follow children for a kinder answer.
  return own ?? { inset: false, trail: `nothing readable behind <${tag}>` };
}

function reactSuspense(file: ts.SourceFile, tag: string): boolean {
  const imported = file.statements.some((statement) => {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier) ||
      statement.moduleSpecifier.text !== "react"
    )
      return false;
    const bindings = statement.importClause?.namedBindings;
    if (bindings && ts.isNamespaceImport(bindings))
      return tag === `${bindings.name.text}.Suspense`;
    return (
      bindings !== undefined &&
      ts.isNamedImports(bindings) &&
      bindings.elements.some(
        (element) =>
          (element.propertyName ?? element.name).text === "Suspense" &&
          element.name.text === tag,
      )
    );
  });
  if (!imported) return false;
  const root = tag.split(".")[0];
  const namesRoot = (name: ts.BindingName): boolean =>
    ts.isIdentifier(name)
      ? name.text === root
      : name.elements.some(
          (element) => ts.isBindingElement(element) && namesRoot(element.name),
        );
  let shadowed = false;
  const visit = (node: ts.Node) => {
    if (
      (ts.isParameter(node) || ts.isVariableDeclaration(node)) &&
      namesRoot(node.name)
    )
      shadowed = true;
    if (
      (ts.isFunctionDeclaration(node) || ts.isClassDeclaration(node)) &&
      node.name?.text === root
    )
      shadowed = true;
    ts.forEachChild(node, visit);
  };
  visit(file);
  return !shadowed;
}

/** What the component itself renders, when this walk can open it. */
function componentVerdict(
  file: ts.SourceFile,
  tag: string,
  walk: Walk,
  seen: ReadonlySet<string>,
): Verdict | undefined {
  const target = componentSource(walk, file, tag);
  if (!target) {
    return undefined;
  }
  const key = `${target.file.fileName}#${target.name}`;
  if (seen.has(key)) {
    return { inset: true, trail: `${tag} (already walked)` };
  }
  const body = bodyOfComponent(target.file, target.name);
  if (!body) {
    return undefined;
  }
  const inner = insetVerdict(
    target.file,
    rootsOfBody(body),
    walk,
    new Set([...seen, key]),
  );
  return { inset: inner.inset, trail: `${tag} → ${inner.trail}` };
}

/** Every address this build answers, from the union that owns them. */
function screenNames(router: ts.SourceFile): string[] {
  return stringsIn(declarationNamed(router, "SCREENS")?.initializer);
}

/** The documented rail-less family, from the set that owns it. */
function railLessNames(nav: ts.SourceFile): string[] {
  return stringsIn(declarationNamed(nav, "RAIL_LESS_SCREENS")?.initializer);
}

/** Screen to the expression that renders it, from the dispatch that owns it. */
function dispatch(
  walk: Walk,
  file: ts.SourceFile,
): ReadonlyMap<string, ts.Expression> {
  const literal = declarationNamed(file, "SCREEN_VIEWS")?.initializer;
  const out = new Map<string, ts.Expression>();
  if (!literal || !ts.isObjectLiteralExpression(literal)) {
    return out;
  }
  for (const property of literal.properties) {
    if (!ts.isPropertyAssignment(property)) {
      continue;
    }
    const key = dispatchKey(walk, file, property.name);
    if (key !== undefined) {
      out.set(key, property.initializer);
    }
  }
  return out;
}

function dispatchKey(
  walk: Walk,
  file: ts.SourceFile,
  name: ts.PropertyName,
): string | undefined {
  if (ts.isStringLiteral(name) || ts.isIdentifier(name)) {
    return name.text;
  }
  if (ts.isComputedPropertyName(name) && ts.isIdentifier(name.expression)) {
    return constantString(walk, file, name.expression.text);
  }
  return undefined;
}

/**
 * The components App renders inside `RaillessFrame`: the pre-session half of
 * the layout exception `RAIL_LESS_SCREENS` states for the shell. A screen
 * routed there has no rail above it and no page head to line up with.
 */
export function raillessComponents(source: ts.SourceFile): ReadonlySet<string> {
  const out = new Set<string>();
  const visit = (node: ts.Node) => {
    if (
      ts.isJsxElement(node) &&
      node.openingElement.tagName.getText() === "RaillessFrame"
    ) {
      const collect = (child: ts.Node) => {
        const tag = ts.isJsxElement(child)
          ? child.openingElement.tagName
          : ts.isJsxSelfClosingElement(child)
            ? child.tagName
            : undefined;
        if (tag && ts.isIdentifier(tag) && /^[A-Z]/.test(tag.text))
          out.add(tag.text);
        ts.forEachChild(child, collect);
      };
      node.children.forEach(collect);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

/** Whether this screen's dispatch renders one of the rail-less frames above. */
function isRaillessRoute(
  entry: ts.Expression,
  railless: ReadonlySet<string>,
): boolean {
  if (!ts.isArrowFunction(entry)) {
    return false;
  }
  return rootsOfBody(entry.body).some(
    (root) =>
      root.kind === "element" && railless.has(root.node.tagName.getText()),
  );
}

type Census = Readonly<{
  screens: string[];
  railLess: ReadonlySet<string>;
  entries: ReadonlyMap<string, ts.Expression>;
  railless: ReadonlySet<string>;
  judged: string[];
  flush: string[];
}>;

const censuses = new WeakMap<CensusTree, Census>();

/** The screens, the lists that exempt some, and the verdict on the rest. */
export function gutterCensus(
  corpus: readonly CensusFile[],
  tree: CensusTree,
): Census {
  const known = censuses.get(tree);
  if (known) return known;
  const walk = walkOver(
    tree,
    selfInsettingClasses(corpus.filter((file) => file.path.endsWith(".css"))),
  );
  const empty = ts.createSourceFile("empty.ts", "", ts.ScriptTarget.Latest);
  const screens = screenNames(walk.at(ROUTER) ?? empty);
  const railLess = new Set(railLessNames(walk.at(NAV) ?? empty));
  const entries = dispatch(walk, walk.at(APP_VIEWS) ?? empty);
  const railless = raillessComponents(walk.at(APP) ?? empty);
  const app = walk.at(APP_SCREENS) ?? empty;
  const judged: string[] = [];
  const flush: string[] = [];
  for (const screen of screens) {
    const entry = entries.get(screen);
    if (
      !entry ||
      railLess.has(screen) ||
      NOT_OURS_TO_INSET.has(screen) ||
      isRaillessRoute(entry, railless)
    ) {
      continue;
    }
    judged.push(screen);
    const verdict = insetVerdict(
      app,
      ts.isArrowFunction(entry) ? rootsOfBody(entry.body) : [],
      walk,
      new Set(),
    );
    if (!verdict.inset) {
      flush.push(`${screen} (src/App.screens.tsx): ${verdict.trail}`);
    }
  }
  const census = { screens, railLess, entries, railless, judged, flush };
  censuses.set(tree, census);
  return census;
}

export const rule: CensusRule = {
  name: "page-gutter-coverage",
  corpus: (tree) => tree.files.filter((file) => file.area === "src"),
  floors: (corpus, tree) => {
    const { screens, railLess, entries, railless, judged } = gutterCensus(
      corpus,
      tree,
    );
    const known = new Set(screens);
    // The dispatch is total over `Screen` by its own type, so a disagreement
    // means this reader missed entries.
    const dispatched = [...entries.keys()].sort(stable).join(" ");
    return [
      ...(screens.length > 0 ? [] : ["SCREENS named no screen"]),
      ...(railLess.size > 0 ? [] : ["RAIL_LESS_SCREENS named no screen"]),
      ...(dispatched === [...screens].sort(stable).join(" ")
        ? []
        : ["SCREEN_VIEWS and SCREENS name different screens"]),
      ...[...railLess, ...NOT_OURS_TO_INSET.keys()]
        .filter((name) => !known.has(name))
        .map((name) => `${name} is exempted and is no screen`),
      ...(railless.size > 0 ? [] : ["App renders nothing in RaillessFrame"]),
      // A census that judged nothing would report the same as a clean tree.
      ...(judged.length > screens.length / 2
        ? []
        : ["half the screens or fewer were judged"]),
    ];
  },
  across: (corpus, tree) => gutterCensus(corpus, tree).flush,
  advice:
    "A screen inside the railed shell renders flush against the scroller. " +
    'Put className="wrap" on the root every branch of the screen draws.',
};
