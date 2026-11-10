// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every screen has a door. A capability nobody can navigate to looks, from a
// user's seat, like one that was never built.
//
// A screen the router answers is reachable if the primary nav carries it, or if
// some shipped surface links to it. A screen opened from outside the app says
// so beside its own entry in `router.tsx`, with a reason.
//
// `SCREENS` and `NAV` are imported from the modules that own them, and the
// waivers are read out of `router.tsx`'s own source, beside their neighbours.

import { join } from "node:path";
import ts from "typescript";
import { resolveRelative } from "../../scripts/lib/source-tree";
import { NAV } from "../../src/app/nav";
import { SCREENS } from "../../src/lib/router";
import { bindingOf } from "../../src/testing/classnames";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

export const routerModule = join(srcDir, "lib", "router.tsx");

// A bare marker would turn the gate off with one keystroke, so a reason is
// required, as `craft:ignore` and `plural-rule:allow` require one.
const WAIVER = /reach:external[ \t]+(?<reason>\S.*)/;

/**
 * The string this expression is, where it is written as one. `as const` and a
 * parenthesis are unwrapped: a constant naming a screen is often written
 * `= "scheduled" as const`.
 */
function literal(node: ts.Node): string | null {
  if (ts.isAsExpression(node) || ts.isParenthesizedExpression(node)) {
    return literal(node.expression);
  }
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    return node.text;
  }
  return null;
}

/**
 * The screen ids named in `router.tsx`'s own list, with any reason waiving one.
 * Read off the syntax tree, since the formatter owns where lines break.
 */
export function waiversIn(parsed: ts.SourceFile): Map<string, string | null> {
  const source = parsed.text;
  const out = new Map<string, string | null>();
  const walk = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "SCREENS"
    ) {
      const array = node.initializer;
      const elements =
        array && ts.isAsExpression(array) ? array.expression : array;
      if (elements && ts.isArrayLiteralExpression(elements)) {
        for (const element of elements.elements) {
          const id = literal(element);
          if (id === null) {
            continue;
          }
          const comments =
            ts.getLeadingCommentRanges(source, element.getFullStart()) ?? [];
          const waiver = comments
            .map((range) => WAIVER.exec(source.slice(range.pos, range.end)))
            .find((match) => match !== null);
          out.set(id, waiver?.groups?.reason?.trim() ?? null);
        }
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parsed);
  return out;
}

/** One name this module imported, and the module that exports it. */
type Import = Readonly<{ from: string; exported: string }>;

/** What one module contributes to the tree's reach, before names are resolved. */
type ModuleReach = Readonly<{
  linked: Set<string>;
  viaName: Set<string>;
  aliases: Map<string, string>;
  imports: Map<string, Import>;
}>;

/**
 * What one module contributes: the doors it opens, and the constants it binds.
 *
 * A door has two spellings: an `#/…` address in markup, and a `screen:`
 * property handed to `navigate` or held as a route. The property is read
 * wherever it appears, so renaming `navigate` cannot defeat the gate.
 *
 * A `screen:` written as an identifier may name a constant in a file not read
 * yet. The name travels out with the imports that could bind it. `doorsIn`
 * resolves it once every module is read, so file order cannot change it.
 */
function readModule(
  parsed: ts.SourceFile,
  screens: ReadonlySet<string>,
): ModuleReach {
  const linked = new Set<string>();
  const viaName = new Set<string>();
  const imports = new Map<string, Import>();
  // The chunk has to begin with the address, bare or joined to an origin. A
  // sentence that names a screen is no door, and counting it would excuse the
  // screen the copy tells a reader to type.
  const address = (text: string): void => {
    const found = /^(?:[a-z][a-z\d+.-]*:\/\/[^/]*)?\/?#\/([a-z-]+)/i.exec(text);
    const id = found?.[1];
    if (id !== undefined && screens.has(id)) {
      linked.add(id);
    }
  };
  // A name is read in the scope it is written in, so a same-named constant in
  // another function cannot answer for it. A name the file never binds is an
  // import, which `doorsIn` follows once every module is read.
  const named = (name: ts.Identifier): void => {
    const bound = bindingOf(name);
    if (bound === undefined) {
      viaName.add(name.text);
      return;
    }
    const value =
      ts.isVariableDeclaration(bound) && bound.initializer
        ? literal(bound.initializer)
        : null;
    if (value !== null && screens.has(value)) {
      linked.add(value);
    }
  };
  const walk = (node: ts.Node): void => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      address(node.text);
    }
    if (ts.isTemplateExpression(node)) {
      // `${origin}/#/room?c=${…}` puts the address in a span, not the head.
      address(node.head.text);
      for (const span of node.templateSpans) {
        address(span.literal.text);
      }
    }
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      const bindings = node.importClause?.namedBindings;
      if (bindings && ts.isNamedImports(bindings)) {
        for (const element of bindings.elements) {
          // The property assignment uses the local name; the other module
          // declares the exported one. They differ under `as`.
          imports.set(element.name.text, {
            from: node.moduleSpecifier.text,
            exported: (element.propertyName ?? element.name).text,
          });
        }
      }
    }
    if (
      ts.isPropertyAssignment(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "screen"
    ) {
      const value = literal(node.initializer);
      if (value !== null && screens.has(value)) {
        linked.add(value);
      }
      if (ts.isIdentifier(node.initializer)) {
        named(node.initializer);
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parsed);
  return {
    linked,
    viaName,
    aliases: exportedScreens(parsed, screens),
    imports,
  };
}

/** The screens a module's top-level constants hold: the only constants of it
 * another module can import. */
function exportedScreens(
  parsed: ts.SourceFile,
  screens: ReadonlySet<string>,
): Map<string, string> {
  const out = new Map<string, string>();
  for (const statement of parsed.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declared of statement.declarationList.declarations) {
      const value =
        ts.isIdentifier(declared.name) && declared.initializer
          ? literal(declared.initializer)
          : null;
      if (value !== null && screens.has(value)) {
        out.set(declared.name.getText(), value);
      }
    }
  }
  return out;
}

/** Every screen the given modules open a door to. */
export function doorsIn(
  files: readonly CensusFile[],
  screens: ReadonlySet<string>,
): Set<string> {
  const modules = new Map<string, ModuleReach>();
  for (const file of files) {
    if (file.source) modules.set(file.path, readModule(file.source, screens));
  }
  const linked = new Set<string>();
  for (const module of modules.values()) {
    for (const id of module.linked) {
      linked.add(id);
    }
  }

  // The screen a name means where it is written: the module's own top-level
  // constants, then its imports. One tree-wide table would let an unrelated
  // file's same-named constant report a door where none exists.
  const bound = (
    path: string,
    name: string,
    seen: Set<string>,
  ): string | undefined => {
    const module = modules.get(path);
    if (module === undefined || seen.has(path)) {
      return undefined;
    }
    seen.add(path);
    const own = module.aliases.get(name);
    if (own !== undefined) {
      return own;
    }
    const imported = module.imports.get(name);
    if (imported === undefined) {
      return undefined;
    }
    const target = resolveRelative(path, imported.from, (file) =>
      modules.has(file),
    );
    return target === null ? undefined : bound(target, imported.exported, seen);
  };

  for (const [path, module] of modules) {
    for (const name of module.viaName) {
      const id = bound(path, name, new Set());
      if (id !== undefined) {
        linked.add(id);
      }
    }
  }
  return linked;
}

/** The screens with no nav row, no link and no reasoned waiver. */
export function unreachedScreens(
  corpus: readonly CensusFile[],
  router: ts.SourceFile,
): string[] {
  const waivers = waiversIn(router);
  const declared = [...waivers.keys()].sort();
  // The waivers are read from source and the subject is imported. Two lists
  // would apply each reason to a screen it was not written for.
  if (declared.join("\n") !== [...SCREENS].sort().join("\n")) {
    return [
      "router.tsx's SCREENS list, read from source, differs from the imported one",
    ];
  }
  const linked = doorsIn(corpus, new Set(SCREENS));
  const carried = new Set(NAV.map((item) => item.screen));
  return SCREENS.filter(
    (screen) =>
      !carried.has(screen) && !linked.has(screen) && !waivers.get(screen),
  );
}

export const rule: CensusRule = {
  name: "reach",
  // Shipped modules only: tests, stories and fixtures link to nothing a reader
  // can press. The router's own list names every screen, so it opens no door.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.source !== undefined &&
        file.path !== routerModule &&
        !/\.(test|testkit|stories|fixtures)\./.test(file.path),
    ),
  // A walk pointed at the wrong tree reports a pass over nothing.
  floors: (corpus, tree) => [
    ...(corpus.length > 100
      ? []
      : ["fewer than 100 shipped modules were read"]),
    ...(tree.at(routerModule)?.source ? [] : ["router.tsx was not read"]),
  ],
  across: (corpus, tree) => {
    const router = tree.at(routerModule)?.source;
    return router ? unreachedScreens(corpus, router) : [];
  },
  advice:
    "A screen the router answers that the nav does not carry and nothing " +
    "links to is reachable only by typing its address. Give it a nav row " +
    "or a link. If it is opened from outside the app, say so beside its " +
    "entry in router.tsx: `// reach:external <reason>`.",
};
