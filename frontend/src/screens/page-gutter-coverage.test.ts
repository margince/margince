// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Fitness function for the page gutter: every screen that renders inside the
// RAILED shell must inset its content from the scroller's edges.
//
// The gutter is `--pageGutter`, and `.wrap` (app/shell.css) is its one spelling
// — the same inset the page head above the screen already takes, which is why a
// screen without it reads as a heading and a body that belong to two different
// pages. The shell does not apply `.wrap`; every screen writes it on its own
// root, and until this file nothing checked that it had. Three screens had not:
// their content sat flush against the edges while their own title sat inset.
//
// ## Why an AST walk and not a render
//
// The alternative was to mount every screen in jsdom and read the computed
// padding. Two things rule it out. A screen mounts only with its queries, its
// session, its route and its translations around it, so the census would become
// a per-screen harness — which is a hand-written list of screens wearing a
// disguise, and the one shape of failure this gate must not have (AGENTS.md:
// "a census that can fail short has already failed"). And jsdom computes no
// cascade for a class it was never handed a stylesheet for, so the reading
// would be of the harness rather than of the product.
//
// The AST reads the three lists from the files that OWN them — the `SCREENS`
// union in app/router.tsx, the `SCREEN_VIEWS` dispatch in App.tsx, and
// `RAIL_LESS_SCREENS` in app/nav.ts — and asserts the first two agree, so a
// destination cannot be added and quietly go unjudged. This is the same
// approach as screens/mutation-variable-coverage.test.ts and
// design-system/native-controls.test.ts, and for the same reason: the rule is
// one sentence about the source, and the TypeScript parser already knows the
// language.
//
// ## What this CANNOT see
//
// - **Amount.** It asks whether the root is inset, not by how much. A screen
//   that pads itself by 2px passes; only `.wrap` is checked for being the
//   product's own number.
// - **The cascade.** A gutter is recognised as `.wrap`, or as a BARE class rule
//   (`.pref-page { padding: … }`) in the tree's CSS. A gutter delivered through
//   a compound selector, an inline style, or a parent's padding is invisible
//   here and reads as absent; a bare class rule inside a media query is read as
//   though it applied at every width.
// - **A className built at runtime.** Class names are read from string literals
//   in the attribute, so a root whose class arrives from a variable or a map
//   lookup reads as having no classes at all.
// - **Anything a component the walk cannot open renders.** An extension unit's
//   screen and a fork's screen come from generated registries; they are named
//   below, with the reason, and the walk stops there.
// - **Whether the inset SURVIVES.** A screen could take the gutter on its root
//   and give it back with a negative margin inside. Nothing static sees that.
// - **A gutter behind a component it cannot open.** The walk follows a wrapper
//   only as far as the wrapper's OWN root, and never into what a caller passed
//   it, because a `.wrap` inside a query gate leaves that gate's pending and
//   refusal states flush against the edge — which is the defect, not a pass. A
//   screen whose gutter genuinely lives inside a component this walk cannot
//   read therefore reads as absent, and the answer is to lift the gutter to the
//   screen's own root rather than to teach the gate an exception.

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const srcRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const ROUTER = join(srcRoot, "app/router.tsx");
const NAV = join(srcRoot, "app/nav.ts");
const APP = join(srcRoot, "App.tsx");

/**
 * The screens whose surface this build does not author, so there is no root of
 * ours to inset. Each is checked below against `SCREENS`, so a renamed or
 * retired destination fails here rather than sitting on as a stale excuse.
 */
const NOT_OURS_TO_INSET: ReadonlyMap<string, string> = new Map([
  [
    "ext",
    "a composed unit's screen arrives from the generated @composition/screens registry; the unit lays its own surface out, and the two branches this build DOES draw for it are inset",
  ],
  [
    "x",
    "a fork's screen arrives from app/custom.ts's registry, which is empty upstream; the fallback this build draws for it is inset",
  ],
]);

// ---------------------------------------------------------------------------
// Reading source
// ---------------------------------------------------------------------------

const parsed = new Map<string, ts.SourceFile>();

function sourceOf(path: string): ts.SourceFile {
  const hit = parsed.get(path);
  if (hit) {
    return hit;
  }
  const file = ts.createSourceFile(
    path,
    readFileSync(path, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    path.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  parsed.set(path, file);
  return file;
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

// ---------------------------------------------------------------------------
// The module graph, as far as a screen needs it
// ---------------------------------------------------------------------------

type Binding = { file: string; exported: string };

const imports = new Map<string, ReadonlyMap<string, Binding>>();

function moduleFor(from: string, specifier: string): string | undefined {
  if (!specifier.startsWith(".")) {
    return undefined;
  }
  const base = resolve(dirname(from), specifier);
  return [
    `${base}.tsx`,
    `${base}.ts`,
    join(base, "index.tsx"),
    join(base, "index.ts"),
  ].find((candidate) => existsSync(candidate));
}

/**
 * Which module each name in a file comes from: static imports, `export … from`
 * re-exports, and the `lazy(routed(() => import("./x").then((m) => ({ default:
 * m.Y }))))` form every routed screen is declared with in App.tsx.
 */
function bindingsOf(file: ts.SourceFile): ReadonlyMap<string, Binding> {
  const hit = imports.get(file.fileName);
  if (hit) {
    return hit;
  }
  const out = new Map<string, Binding>();
  for (const statement of file.statements) {
    if (ts.isImportDeclaration(statement)) {
      readImport(file, statement, out);
    } else if (ts.isExportDeclaration(statement)) {
      readReExport(file, statement, out);
    } else if (ts.isVariableStatement(statement)) {
      readLazyImport(file, statement, out);
    }
  }
  imports.set(file.fileName, out);
  return out;
}

function readImport(
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
  const target = moduleFor(file.fileName, statement.moduleSpecifier.text);
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

/** `export { LeadsScreen } from "./leads.list"` — a screen's file can be a door. */
function readReExport(
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
  const target = moduleFor(file.fileName, specifier.text);
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
  file: ts.SourceFile,
  statement: ts.VariableStatement,
  out: Map<string, Binding>,
): void {
  for (const declaration of statement.declarationList.declarations) {
    if (!ts.isIdentifier(declaration.name) || !declaration.initializer) {
      continue;
    }
    const specifier = dynamicImportSpecifier(declaration.initializer);
    const target = specifier ? moduleFor(file.fileName, specifier) : undefined;
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
function bodyOfComponent(
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
 * The file that actually DECLARES a component, following as many doors as it
 * takes: App.tsx's `lazy` constant is one, and a screen file that re-exports its
 * list from a sibling (`export { LeadsScreen } from "./leads.list"`) is another.
 * Stopping at the first hop read a re-export as an unreadable component.
 */
function componentSource(
  file: ts.SourceFile,
  name: string,
  hops = 0,
): { file: ts.SourceFile; name: string } | undefined {
  if (bodyOfComponent(file, name)) {
    return { file, name };
  }
  const binding = bindingsOf(file).get(name);
  // The cap is what stops a re-export cycle from walking forever; no chain in
  // this tree is more than two doors long.
  if (!binding?.file.startsWith(srcRoot) || hops >= 8) {
    return undefined;
  }
  return componentSource(sourceOf(binding.file), binding.exported, hops + 1);
}

/** `export const EXTENSION_SCREEN = "ext"`, followed through the import. */
function constantString(file: ts.SourceFile, name: string): string | undefined {
  const local = declarationNamed(file, name);
  if (local?.initializer && ts.isStringLiteralLike(local.initializer)) {
    return local.initializer.text;
  }
  const binding = bindingsOf(file).get(name);
  return binding
    ? constantString(sourceOf(binding.file), binding.exported)
    : undefined;
}

// ---------------------------------------------------------------------------
// What a component renders at its root
// ---------------------------------------------------------------------------

type Root =
  | { kind: "element"; node: ts.JsxOpeningLikeElement }
  | { kind: "nothing" }
  | { kind: "unreadable"; text: string };

function rootsOfBody(body: ts.Node): Root[] {
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
  // A render prop — `<QueryGate>{(session) => <Workbench …/>}</QueryGate>` — is
  // a child that draws, so what it returns is what stands there.
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

// ---------------------------------------------------------------------------
// Is this element inset?
// ---------------------------------------------------------------------------

const GUTTER_CLASS = "wrap";

function classesOn(element: ts.JsxOpeningLikeElement): string[] {
  for (const attribute of element.attributes.properties) {
    if (
      ts.isJsxAttribute(attribute) &&
      ts.isIdentifier(attribute.name) &&
      attribute.name.text === "className"
    ) {
      return stringsIn(attribute.initializer).flatMap((value) =>
        value.split(/\s+/).filter(Boolean),
      );
    }
  }
  return [];
}

/**
 * Bare class selectors whose rule declares a non-zero horizontal padding —
 * the public pages that stand outside the shell's column and spell their own
 * inset (`.pref-page`). Bare only: a rule reached through a compound selector
 * describes a class in a context, and this asks about the class itself.
 */
function selfInsettingClasses(): ReadonlySet<string> {
  const out = new Set<string>();
  for (const file of cssFilesUnder(srcRoot)) {
    // Comments out first: they hold prose about padding, and braces.
    const text = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    for (const [, selector, block] of text.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      const name = /^\s*\.([A-Za-z0-9_-]+)\s*$/.exec(selector)?.[1];
      if (name && declaresHorizontalPadding(block)) {
        out.add(name);
      }
    }
  }
  return out;
}

function cssFilesUnder(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return cssFilesUnder(path);
    }
    return entry.name.endsWith(".css") ? [path] : [];
  });
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
  // Split on the spaces BETWEEN values, never on one inside `var(--a, 10px)`.
  const parts = value.split(/\s+(?![^(]*\))/);
  // Only the shorthand puts the horizontal half second; every longhand that
  // reaches here already names a horizontal edge.
  return property === "padding" && parts.length > 1 ? parts[1] : parts[0];
}

function isZero(value: string | undefined): boolean {
  return value === undefined || /^0[a-z%]*$/i.test(value);
}

type Verdict = { inset: boolean; trail: string };

function insetVerdict(
  file: ts.SourceFile,
  roots: readonly Root[],
  insetting: ReadonlySet<string>,
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
    const verdict = elementVerdict(file, root.node, insetting, seen);
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
  insetting: ReadonlySet<string>,
  seen: ReadonlySet<string>,
): Verdict {
  const tag = element.tagName.getText();
  if (/^[a-z]/.test(tag)) {
    const classes = classesOn(element);
    if (classes.includes(GUTTER_CLASS)) {
      return { inset: true, trail: `<${tag} class="${classes.join(" ")}">` };
    }
    if (classes.some((name) => insetting.has(name))) {
      return { inset: true, trail: `<${tag}> insets itself in CSS` };
    }
    return {
      inset: false,
      trail: `root <${tag} class="${classes.join(" ")}"> takes no page gutter`,
    };
  }
  const own = componentVerdict(file, tag, insetting, seen);
  if (own?.inset) {
    return own;
  }
  // A wrapper that insets only what it WRAPS is not an inset screen: the gutter
  // has to be on the root every branch of that wrapper draws, or the branches
  // it draws instead — a pending body, a refusal — reach the reader flush
  // against the scroller while the loaded page looks correct. That is the
  // defect this gate exists for, so the walk does not follow children looking
  // for a kinder answer.
  return own ?? { inset: false, trail: `nothing readable behind <${tag}>` };
}

/** What the component itself renders, when this walk can open it. */
function componentVerdict(
  file: ts.SourceFile,
  tag: string,
  insetting: ReadonlySet<string>,
  seen: ReadonlySet<string>,
): Verdict | undefined {
  const target = componentSource(file, tag);
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
    insetting,
    new Set([...seen, key]),
  );
  return { inset: inner.inset, trail: `${tag} → ${inner.trail}` };
}

// ---------------------------------------------------------------------------
// The census
// ---------------------------------------------------------------------------

/** Every address this build answers, from the union that owns them. */
function screenNames(): string[] {
  return stringsIn(declarationNamed(sourceOf(ROUTER), "SCREENS")?.initializer);
}

/** The documented rail-less family, from the set that owns it. */
function railLessNames(): string[] {
  return stringsIn(
    declarationNamed(sourceOf(NAV), "RAIL_LESS_SCREENS")?.initializer,
  );
}

/** Screen → the expression that renders it, from the dispatch that owns it. */
function dispatch(): ReadonlyMap<string, ts.Expression> {
  const file = sourceOf(APP);
  const literal = declarationNamed(file, "SCREEN_VIEWS")?.initializer;
  const out = new Map<string, ts.Expression>();
  if (!literal || !ts.isObjectLiteralExpression(literal)) {
    return out;
  }
  for (const property of literal.properties) {
    if (!ts.isPropertyAssignment(property)) {
      continue;
    }
    const key = dispatchKey(file, property.name);
    if (key !== undefined) {
      out.set(key, property.initializer);
    }
  }
  return out;
}

function dispatchKey(
  file: ts.SourceFile,
  name: ts.PropertyName,
): string | undefined {
  if (ts.isStringLiteral(name) || ts.isIdentifier(name)) {
    return name.text;
  }
  if (ts.isComputedPropertyName(name) && ts.isIdentifier(name.expression)) {
    return constantString(file, name.expression.text);
  }
  return undefined;
}

/**
 * The components App renders inside `RaillessFrame`, which is the pre-session
 * half of the same layout exception `RAIL_LESS_SCREENS` states for the shell.
 * Derived rather than listed: a screen routed there has no rail above it and
 * therefore no page head to line up with.
 */
function raillessComponents(): ReadonlySet<string> {
  const out = new Set<string>();
  const visit = (node: ts.Node) => {
    if (
      ts.isJsxElement(node) &&
      node.openingElement.tagName.getText() === "RaillessFrame"
    ) {
      for (const root of node.children.flatMap(rootsOfChild)) {
        if (root.kind === "element") {
          out.add(root.node.tagName.getText());
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceOf(APP));
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

describe("page gutter", () => {
  const screens = screenNames();
  const railLess = new Set(railLessNames());
  const entries = dispatch();

  it("reads a whole census, not a partial one", () => {
    expect(screens.length).toBeGreaterThan(0);
    expect(railLess.size).toBeGreaterThan(0);
    // The dispatch is total over `Screen` by its own type, so a disagreement
    // here means this file's reader missed entries rather than that the product
    // lost a route — and a reader that misses entries reports PASS for screens
    // it never opened.
    expect([...entries.keys()].sort()).toEqual([...screens].sort());
  });

  it("names only screens that still exist", () => {
    const known = new Set(screens);
    expect([...railLess].filter((name) => !known.has(name))).toEqual([]);
    expect(
      [...NOT_OURS_TO_INSET.keys()].filter((name) => !known.has(name)),
    ).toEqual([]);
  });

  it("insets every screen that renders inside the railed shell", () => {
    const insetting = selfInsettingClasses();
    const railless = raillessComponents();
    expect(railless.size).toBeGreaterThan(0);
    const app = sourceOf(APP);
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
        insetting,
        new Set(),
      );
      if (!verdict.inset) {
        flush.push(
          `${screen} (${relative(srcRoot, app.fileName)}): ${verdict.trail}`,
        );
      }
    }
    // A census that judged nothing would report the same word as a clean tree.
    expect(judged.length).toBeGreaterThan(screens.length / 2);
    expect(flush).toEqual([]);
  });
});
