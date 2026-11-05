// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import {
  bindingOf,
  classVariants,
  declarationOf,
  type Given,
} from "../../src/testing/classnames";
import { srcDir } from "./dialoglayout.corpus";
import { bindings, declarations } from "./dialoglayout.names";
import { fnOf, givenTo } from "./dialoglayout.props";
import { resolveRelative, sourceFileAt } from "./source-tree";

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

export type Member = ts.JsxAttributeLike | ts.ObjectLiteralElementLike;

/** A member's value as written, a JSX attribute's expression unwrapped. */
export const initOf = (m: Member) => {
  if (ts.isShorthandPropertyAssignment(m)) return m.name;
  if (ts.isPropertyAssignment(m)) return m.initializer;
  const init = ts.isJsxAttribute(m) ? m.initializer : undefined;
  return init && ts.isJsxExpression(init) ? init.expression : init;
};

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
  const e = a && initOf(a);
  if (e && ts.isTemplateExpression(e)) {
    return e.templateSpans.map((span) => span.expression.getText());
  }
  return e ? [e.getText()] : [];
}

/** Each branch of the class list an element's `className` produces. */
export const variantsOf = (n: ts.Node, given?: Given) =>
  classVariants(
    n.getSourceFile(),
    attr(n, "className")?.initializer,
    16,
    given,
  );

/** Every class any branch of the element's `className` can carry. */
export const classes = (n: ts.Node) => [...new Set(variantsOf(n).flat())];

export const within = (n: ts.Node, outer: ts.Node) =>
  n.getSourceFile() === outer.getSourceFile() &&
  outer.pos <= n.pos &&
  n.end <= outer.end;

export { declarations } from "./dialoglayout.names";
export {
  domainOf,
  fnOf,
  givenTo,
  literalTypes,
  presetOf,
  propReadBy,
  spreads,
} from "./dialoglayout.props";

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

// Where a component tag is defined, through imports, re-exports, renames and
// `const B = A`.
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

const PASS_THROUGH = new Set(["memo", "forwardRef"]);
const called = (x: ts.Expression) =>
  ts.isIdentifier(x)
    ? x.text
    : ts.isPropertyAccessExpression(x)
      ? x.name.text
      : "";

// The component a wrapper hands through, when it names one. Both entry points
// ask, so an alias of a memo resolves as far as the memo itself does.
function handedThrough(x: ts.Expression): ts.Identifier | undefined {
  if (!ts.isCallExpression(x) || !PASS_THROUGH.has(called(x.expression))) {
    return undefined;
  }
  const [first] = x.arguments;
  return first && ts.isIdentifier(first) ? first : undefined;
}

// The module and export a `lazy` loader imports: `default`, or the member its
// `.then((m) => ({ default: m.X }))` hands on. A helper wrapping the loader
// (`lazy(routed(() => import(…)))`) is read through.
function loadedBy(
  x: ts.Expression,
): { spec: string; name: string } | undefined {
  if (!ts.isCallExpression(x) || called(x.expression) !== "lazy") {
    return undefined;
  }
  const inside = x.arguments.flatMap((a) => [a, ...descendants(a)]);
  const load = inside.find(
    (n): n is ts.CallExpression =>
      ts.isCallExpression(n) &&
      n.expression.kind === ts.SyntaxKind.ImportKeyword,
  );
  const [spec] = load?.arguments ?? [];
  if (!spec || !ts.isStringLiteralLike(spec)) return undefined;
  const picked = inside.find(
    (n): n is ts.PropertyAssignment =>
      ts.isPropertyAssignment(n) &&
      n.name.getText() === "default" &&
      ts.isPropertyAccessExpression(n.initializer),
  );
  const member = picked?.initializer;
  const name =
    member && ts.isPropertyAccessExpression(member)
      ? member.name.text
      : "default";
  return { spec: spec.text, name };
}

// The component a wrapper call stands for. `memo(X)` and `forwardRef(X)` hand
// X's own element tree through, and reading the binding as an element of its
// own judged none of X's rows. `lazy` stands for what its loader imports.
function wrappedBy(
  source: ts.SourceFile,
  x: ts.Expression,
  hops: number,
): { def: ts.Node | undefined } | undefined {
  const handed = handedThrough(x);
  if (handed) return { def: alias(handed, hops) };
  const lazy = loadedBy(x);
  const file = lazy && moduleFor(source.fileName, lazy.spec);
  return lazy && file
    ? { def: follow(sourceFileAt(file), lazy.name, hops) }
    : undefined;
}

// What an alias names, read from the scope it sits in. A name alone reads the
// file's top level: for `const Glyph = Icon` in a component taking an `Icon`
// prop, that is a top-level `Icon`, not the prop.
//
// A name the file never binds is an import, still followable. A parameter or a
// destructured name is a binding this walk cannot enter, and answers nothing.
function alias(name: ts.Identifier, hops: number): ts.Node | undefined {
  const scoped = declarationOf(name);
  if (!scoped) {
    const source = name.getSourceFile();
    return !bindingOf(name) && bindings(source).has(name.text)
      ? follow(source, name.text, hops)
      : undefined;
  }
  const next = ts.isVariableDeclaration(scoped)
    ? scoped.initializer
    : undefined;
  const source = name.getSourceFile();
  const wrapped =
    next && hops > 0 ? wrappedBy(source, next, hops - 1) : undefined;
  if (wrapped) return wrapped.def;
  return next && ts.isIdentifier(next) && hops > 0
    ? alias(next, hops - 1)
    : scoped;
}

function follow(
  source: ts.SourceFile,
  name: string,
  hops: number,
): ts.Node | undefined {
  const local = declarations(source).get(name);
  const link = bindings(source).get(name);
  const to = local && ts.isVariableDeclaration(local) && local.initializer;
  if (to && ts.isIdentifier(to) && hops > 0) return alias(to, hops - 1);
  const wrapped = to && hops > 0 ? wrappedBy(source, to, hops - 1) : undefined;
  if (wrapped) return wrapped.def;

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

export const defOf = (n: ts.Node) => {
  const name = opening(n)?.tagName;
  if (!upper(n) || !name) return undefined;
  return ts.isIdentifier(name)
    ? definitionOf(name)
    : definition(n.getSourceFile(), tag(n));
};

// What a name means where it is written: its own scope first, then the file's
// top level and imports, which `definition` caches.
export const definitionOf = (name: ts.Identifier) =>
  bindingOf(name)
    ? alias(name, 6)
    : definition(name.getSourceFile(), name.text);

/** The key of what `name` is in `design-system/<file>.tsx`. */
export function primitiveKey(file: string, name: string): string {
  const path = join(srcDir, "design-system", `${file}.tsx`);
  const def = definition(sourceFileAt(path), name);
  if (!def) throw new Error(`${path} no longer defines ${name}`);
  return keyOf(def);
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
