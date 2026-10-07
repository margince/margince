// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHICH MODULES THE SHIPPED BUNDLE ACTUALLY LOADS, and the entry points it
// starts from.
//
// Every census over the frontend source has the same trap: a story, a test or a
// fixture NAMES the thing it is asked about, so a walk over `src/**` counts
// vouching-for-itself as usage. A catalog key rendered only by a story reads as
// live; a CSS rule reached only by a story reads as live. Both are the same
// question — is anything the user runs going to touch this — and both need the
// same corpus.
//
// Shared rather than copied, because the two censuses that ask it would
// otherwise drift: the one with the weaker corpus goes quiet first, and a census
// that under-reports is indistinguishable from a clean tree.
//
// The entry points are DERIVED, never listed: the index.html documents Vite
// builds, the surface package.json exports to extension units, and the modules
// the PWA build names in its own script. A surface added tomorrow is an entry
// the day it is added.

import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import {
  filesMatching,
  filesUnder,
  moduleAt,
  moduleSpecifiers,
  parseSource,
  resolveRelative,
} from "./source-tree";

export type SourceTree = ReadonlyMap<string, string>;

export type Program = {
  readonly tree: SourceTree;
  /** Bare specifiers the bundler maps onto a path in the tree, before suffixes. */
  readonly aliases: ReadonlyMap<string, string>;
  /** Files whose literals are the keys themselves. */
  readonly catalogs: ReadonlySet<string>;
};

export type Module = {
  readonly specifiers: readonly string[];
  readonly globs: readonly string[];
  readonly literals: readonly string[];
  readonly stems: readonly string[];
};

const SILENT: Module = { specifiers: [], globs: [], literals: [], stems: [] };

// A dot makes a template head a key stem; without one it is a class name or a
// URL, and taking it as a stem would vouch for the whole catalog.
const KEY_STEM = /^[A-Za-z0-9_]+\.[A-Za-z0-9_.]*$/;

// Vite's queries that hand over text or a URL; any other query still runs it.
const TEXT_QUERY = /\?(?:raw|url|inline)$/;

function resolveModule(
  program: Program,
  from: string,
  specifier: string,
): string | null {
  if (TEXT_QUERY.test(specifier)) return null;
  const path = specifier.replace(/\?.*$/, "");
  const inTree = (candidate: string): boolean => program.tree.has(candidate);
  const aliased = program.aliases.get(path);
  return aliased === undefined
    ? resolveRelative(from, path, inTree)
    : moduleAt(aliased, inTree);
}

function globMatches(tree: SourceTree, from: string, glob: string): string[] {
  if (!glob.startsWith(".")) return [];
  const pattern = resolve(dirname(from), glob)
    .split(/(\*\*\/|\*)/)
    .map((part) => {
      if (part === "**/") return "(?:.*/)?";
      if (part === "*") return "[^/]*";
      return part.replace(/[.+?^${}()|[\]\\]/g, "\\$&");
    })
    .join("");
  const matcher = new RegExp(`^${pattern}$`);
  return [...tree.keys()].filter((path) => matcher.test(path));
}

function stringsOf(node: ts.Node | undefined): string[] {
  if (node === undefined) return [];
  if (ts.isStringLiteralLike(node)) return [node.text];
  return ts.isArrayLiteralExpression(node)
    ? node.elements.flatMap((element) => stringsOf(element))
    : [];
}

function isMetaGlob(callee: ts.Expression): boolean {
  return (
    ts.isPropertyAccessExpression(callee) &&
    callee.name.text === "glob" &&
    ts.isMetaProperty(callee.expression) &&
    callee.expression.keywordToken === ts.SyntaxKind.ImportKeyword
  );
}

// A property name is a lookup key, not copy handed to `t()`.
function isPropertyName(node: ts.Node): boolean {
  const parent = node.parent;
  return (
    (ts.isPropertyAssignment(parent) ||
      ts.isPropertySignature(parent) ||
      ts.isPropertyDeclaration(parent) ||
      ts.isMethodDeclaration(parent) ||
      ts.isEnumMember(parent)) &&
    parent.name === node
  );
}

// Comments are trivia and types are erased, so a key named only there is unseen.
function readModule(path: string, text: string): Module {
  const source = parseSource(path, text);
  const found: { [field in keyof Module]: string[] } = {
    specifiers: moduleSpecifiers(source, "values"),
    globs: [],
    literals: [],
    stems: [],
  };
  const walk = (node: ts.Node): void => {
    if (ts.isTypeNode(node)) return;
    if (ts.isCallExpression(node) && isMetaGlob(node.expression)) {
      found.globs.push(...stringsOf(node.arguments[0]));
    }
    if (ts.isStringLiteralLike(node) && !isPropertyName(node)) {
      found.literals.push(node.text);
    }
    if (ts.isTemplateExpression(node) && KEY_STEM.test(node.head.text)) {
      found.stems.push(node.head.text);
    }
    ts.forEachChild(node, walk);
  };
  walk(source);
  return found;
}

export type ModuleGraph = {
  /** Every module the roots load, each parsed once however often reached. */
  readonly reach: (roots: readonly string[]) => ReadonlyMap<string, Module>;
};

export function moduleGraph(program: Program): ModuleGraph {
  const parsed = new Map<string, Module>();
  const moduleOf = (path: string): Module | undefined => {
    const text = program.tree.get(path);
    if (text === undefined) return undefined;
    const known = parsed.get(path);
    if (known !== undefined) return known;
    const module = program.catalogs.has(path) ? SILENT : readModule(path, text);
    parsed.set(path, module);
    return module;
  };
  // What one module pulls in: its resolved imports, plus every file its meta
  // globs match. Separated from the walk because the walk is about visiting
  // once and this is about what a visit finds.
  const edgesFrom = (path: string, module: Module): string[] => [
    ...module.specifiers.flatMap((specifier) => {
      const target = resolveModule(program, path, specifier);
      return target === null ? [] : [target];
    }),
    ...module.globs.flatMap((glob) => globMatches(program.tree, path, glob)),
  ];
  const reach = (roots: readonly string[]): ReadonlyMap<string, Module> => {
    const reached = new Map<string, Module>();
    const queue = [...roots];
    for (let path = queue.pop(); path !== undefined; path = queue.pop()) {
      const module = reached.has(path) ? undefined : moduleOf(path);
      if (module === undefined) continue;
      reached.set(path, module);
      queue.push(...edgesFrom(path, module));
    }
    return reached;
  };
  return { reach };
}

export const FRONTEND_ROOT = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
);
export const SRC_ROOT = join(FRONTEND_ROOT, "src");

// vite.config.ts names no build input, so Vite builds its default: the root
// index.html. Each MCP view under src/ is built from its own index.html.
export function documentEntries(): string[] {
  const documents = [
    join(FRONTEND_ROOT, "index.html"),
    ...filesMatching(SRC_ROOT, /^index\.html$/),
  ];
  return documents.flatMap((document) =>
    [...readFileSync(document, "utf8").matchAll(/<script\b([^>]*)>/g)]
      .map(([, attributes]) => attributes)
      .filter((attributes) => /\btype="module"/.test(attributes))
      .flatMap((attributes) => [...attributes.matchAll(/\bsrc="([^"]+)"/g)])
      .map(([, source]) =>
        source.startsWith("/")
          ? join(FRONTEND_ROOT, source)
          : resolve(dirname(document), source),
      ),
  );
}

/** The host surface extension units import, as package.json exports it. */
export function surfaceEntries(): string[] {
  const manifest: unknown = JSON.parse(
    readFileSync(join(FRONTEND_ROOT, "package.json"), "utf8"),
  );
  const exported =
    typeof manifest === "object" && manifest !== null && "exports" in manifest
      ? manifest.exports
      : null;
  return typeof exported === "object" && exported !== null
    ? Object.values(exported).flatMap((target: unknown) =>
        typeof target === "string" ? [resolve(FRONTEND_ROOT, target)] : [],
      )
    : [];
}

/**
 * The modules the PWA build loads outside the app bundle: the offline page it
 * renders at build time and the offline script it bundles, read off the script
 * that names them so a new one is an entry the day it is added.
 */
export function buildScriptEntries(): string[] {
  const script = readFileSync(
    join(FRONTEND_ROOT, "scripts", "vite-pwa.ts"),
    "utf8",
  );
  return [...script.matchAll(/resolve\(FRONTEND, "(src\/[^"]+\.tsx?)"\)/g)].map(
    ([, source]) => join(FRONTEND_ROOT, source),
  );
}

// Not vite.config.ts: loading it runs the composition switch. tsconfig.app.json
// carries the same vanilla-lane mapping for the compiler.
export function aliases(): Map<string, string> {
  const file = join(FRONTEND_ROOT, "tsconfig.app.json");
  const { config } = ts.readConfigFile(file, ts.sys.readFile);
  const { options } = ts.parseJsonConfigFileContent(
    config,
    ts.sys,
    FRONTEND_ROOT,
  );
  const base = options.baseUrl ?? FRONTEND_ROOT;
  return new Map(
    Object.entries(options.paths ?? {}).flatMap(([specifier, targets]) =>
      targets
        .slice(0, 1)
        .map((target): [string, string] => [specifier, resolve(base, target)]),
    ),
  );
}

/**
 * The modules the shipped bundle loads, keyed by absolute path.
 *
 * The whole wiring in one call, because a caller that assembled the tree, the
 * aliases and the entry list for itself would be free to assemble a DIFFERENT
 * one — and the census with the looser corpus is the one that goes quiet.
 */
export function reachedModules(
  extra: { readonly catalogs?: ReadonlySet<string> } = {},
): ReadonlyMap<string, Module> {
  const tree: SourceTree = new Map(
    filesUnder(SRC_ROOT).map((path): [string, string] => [
      path,
      readFileSync(path, "utf8"),
    ]),
  );
  const graph = moduleGraph({
    tree,
    aliases: aliases(),
    catalogs: extra.catalogs ?? new Set<string>(),
  });
  return graph.reach([
    ...documentEntries(),
    ...surfaceEntries(),
    ...buildScriptEntries(),
  ]);
}
