// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  filesMatching,
  filesUnder,
  moduleAt,
  moduleSpecifiers,
  parseSource,
  resolveRelative,
} from "../../scripts/lib/source-tree";
import { en } from "./en";
import { LOCALES } from "./index";

// A key is live only when a module the shipped bundle loads names it: a story,
// a test, a fixture or a module nothing imports vouches for nothing.

type SourceTree = ReadonlyMap<string, string>;

type Program = {
  readonly tree: SourceTree;
  /** Bare specifiers the bundler maps onto a path in the tree, before suffixes. */
  readonly aliases: ReadonlyMap<string, string>;
  /** Files whose literals are the keys themselves. */
  readonly catalogs: ReadonlySet<string>;
};

type Module = {
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

type ModuleGraph = {
  /** Every module the roots load, each parsed once however often reached. */
  readonly reach: (roots: readonly string[]) => ReadonlyMap<string, Module>;
};

function moduleGraph(program: Program): ModuleGraph {
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
  const reach = (roots: readonly string[]): ReadonlyMap<string, Module> => {
    const reached = new Map<string, Module>();
    const queue = [...roots];
    for (let path = queue.pop(); path !== undefined; path = queue.pop()) {
      const module = reached.has(path) ? undefined : moduleOf(path);
      if (module === undefined) continue;
      reached.set(path, module);
      for (const specifier of module.specifiers) {
        const target = resolveModule(program, path, specifier);
        if (target !== null) queue.push(target);
      }
      for (const glob of module.globs) {
        queue.push(...globMatches(program.tree, path, glob));
      }
    }
    return reached;
  };
  return { reach };
}

/**
 * Keys no loaded module renders, literally, under a template stem, or as the
 * arm of a plural pair whose base it names.
 */
function orphanKeys(
  keys: readonly string[],
  modules: Iterable<Module>,
): string[] {
  const loaded = [...modules];
  const literals = new Set(loaded.flatMap((module) => module.literals));
  const stems = [...new Set(loaded.flatMap((module) => module.stems))];
  const catalog = new Set(keys);
  // Only the two arms of a full pair: half a pair is the orphan this finds.
  const underPluralBase = (key: string): boolean => {
    const arm = ["_one", "_other"].find((suffix) => key.endsWith(suffix));
    if (arm === undefined) return false;
    const base = key.slice(0, -arm.length);
    return (
      catalog.has(`${base}_one`) &&
      catalog.has(`${base}_other`) &&
      literals.has(base)
    );
  };
  return keys.filter(
    (key) =>
      !literals.has(key) &&
      !underPluralBase(key) &&
      !stems.some((stem) => key.startsWith(stem)),
  );
}

describe("the orphan finder, over a planted tree", () => {
  const MAIN = "/app/src/main.tsx";

  const verdict = (
    files: Record<string, string>,
    keys: readonly string[],
  ): string[] => {
    const graph = moduleGraph({
      tree: new Map(Object.entries(files)),
      aliases: new Map([["@alias/copy", "/app/src/aliased"]]),
      catalogs: new Set(),
    });
    return orphanKeys(keys, graph.reach([MAIN]).values());
  };

  it("counts a key the entry renders", () => {
    expect(verdict({ [MAIN]: `t("a.live");` }, ["a.live"])).toEqual([]);
  });

  it("refuses a key only a story renders", () => {
    const files = {
      [MAIN]: `import { Card } from "./card";`,
      "/app/src/card.tsx": `export const Card = () => null;`,
      "/app/src/card.stories.tsx": `import { Card } from "./card"; t("a.story");`,
    };
    expect(verdict(files, ["a.story"])).toEqual(["a.story"]);
  });

  it("refuses a key only an erased type import reaches", () => {
    const files = {
      [MAIN]: [
        `import type { A } from "./a";`,
        `export type { C } from "./c";`,
        `import { type B } from "./b";`,
      ].join("\n"),
      "/app/src/a.ts": `export type A = 1; t("a.typeImport");`,
      "/app/src/c.ts": `export type C = 1; t("c.typeExport");`,
      "/app/src/b.ts": `export type B = 1; t("b.inlineType");`,
    };
    const keys = ["a.typeImport", "c.typeExport", "b.inlineType"];
    expect(verdict(files, keys)).toEqual(["a.typeImport", "c.typeExport"]);
  });

  it("refuses a key named only as a type or a property name", () => {
    const files = {
      [MAIN]: [
        `type K = "a.asType";`,
        `const table = { "a.asName": 1, b: "a.asValue" };`,
      ].join("\n"),
    };
    const keys = ["a.asType", "a.asName", "a.asValue"];
    expect(verdict(files, keys)).toEqual(["a.asType", "a.asName"]);
  });

  it("refuses a key only a module nothing imports renders", () => {
    const files = { [MAIN]: ``, "/app/src/stray.tsx": `t("a.stray");` };
    expect(verdict(files, ["a.stray"])).toEqual(["a.stray"]);
  });

  it("refuses a key only a comment names", () => {
    const files = { [MAIN]: `// t("a.line")\n/* "a.block" */ t("a.live");` };
    expect(verdict(files, ["a.line", "a.block", "a.live"])).toEqual([
      "a.line",
      "a.block",
    ]);
  });

  it("takes template stems only from loaded modules", () => {
    const files = {
      [MAIN]: `t(\`live.\${status}\`);`,
      "/app/src/stray.tsx": `t(\`stray.\${status}\`);`,
    };
    expect(verdict(files, ["live.open", "stray.open"])).toEqual(["stray.open"]);
  });

  it("follows a dynamic import, a re-export chain, an alias and a glob", () => {
    const files = {
      [MAIN]: [
        `const Lazy = lazy(() => import("./lazy"));`,
        `import { Deep } from "./barrel";`,
        `import copy from "@alias/copy";`,
        `const screens = import.meta.glob("./custom/*/screen.tsx");`,
      ].join("\n"),
      "/app/src/lazy.tsx": `t("a.lazy");`,
      "/app/src/barrel/index.ts": `export * from "./middle.ts";`,
      "/app/src/barrel/middle.ts": `export { Deep } from "../deep";`,
      "/app/src/deep.tsx": `t("a.reexported");`,
      "/app/src/aliased.ts": `t("a.aliased");`,
      "/app/src/custom/one/screen.tsx": `t("a.globbed");`,
    };
    const keys = ["a.lazy", "a.reexported", "a.aliased", "a.globbed"];
    expect(verdict(files, keys)).toEqual([]);
  });

  it("does not run a module imported as text or a URL, but runs a worker", () => {
    const files = {
      [MAIN]: [
        `import raw from "./raw.ts?raw";`,
        `import url from "./url.ts?url";`,
        `import Worker from "./work.ts?worker";`,
      ].join("\n"),
      "/app/src/raw.ts": `t("a.raw");`,
      "/app/src/url.ts": `t("a.url");`,
      "/app/src/work.ts": `t("a.worker");`,
    };
    const keys = ["a.raw", "a.url", "a.worker"];
    expect(verdict(files, keys)).toEqual(["a.raw", "a.url"]);
  });

  it("counts a plural pair by its base only where a loaded module names it", () => {
    const files = {
      [MAIN]: `plural("live.count", n);`,
      "/app/src/main.test.ts": `plural("dead.count", n);`,
    };
    const keys = [
      "live.count_one",
      "live.count_other",
      "dead.count_one",
      "dead.count_other",
    ];
    expect(verdict(files, keys)).toEqual([
      "dead.count_one",
      "dead.count_other",
    ]);
  });

  it("does not stretch a plural base past its two arms", () => {
    const files = { [MAIN]: `plural("a.count", n);` };
    const keys = ["a.count_one", "a.count_other", "a.count_extra"];
    expect(verdict(files, keys)).toEqual(["a.count_extra"]);
  });
});

const SRC_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const FRONTEND_ROOT = resolve(SRC_ROOT, "..");

// vite.config.ts names no build input, so Vite builds its default: the root
// index.html. Each MCP view under src/ is built from its own index.html.
function documentEntries(): string[] {
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
function surfaceEntries(): string[] {
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
function buildScriptEntries(): string[] {
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
function aliases(): Map<string, string> {
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

describe("catalog keys against the bundle that renders them", () => {
  const tree: SourceTree = new Map(
    filesUnder(SRC_ROOT).map((path): [string, string] => [
      path,
      readFileSync(path, "utf8"),
    ]),
  );
  const catalogs = new Set(
    LOCALES.map((locale) => join(SRC_ROOT, "i18n", `${locale}.ts`)),
  );
  const graph = moduleGraph({ tree, aliases: aliases(), catalogs });
  const entries = [
    ...documentEntries(),
    ...surfaceEntries(),
    ...buildScriptEntries(),
  ];
  const mounted = graph.reach(entries);
  const keys = Object.keys(en);

  it("derives an entry point from every owner that declares one", () => {
    expect(documentEntries().length).toBeGreaterThan(1);
    expect(surfaceEntries().length).toBeGreaterThan(0);
    expect(buildScriptEntries().length).toBeGreaterThan(0);
    const missing = [...entries, ...catalogs].filter((path) => !tree.has(path));
    expect(missing, "declared, but not a file in src").toEqual([]);
  });

  it("every key is rendered by a module the bundle loads", () => {
    const orphans = orphanKeys(keys, mounted.values());
    expect(
      orphans,
      `keys translated three times and rendered nowhere: ${orphans.join(", ")}`,
    ).toEqual([]);
  });
});
