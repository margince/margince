// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The extension frontend import gate: a unit's screen may reach the core ONLY
// through the published surface, and may reach npm only through what its own
// package declares.
//
// This is the frontend's answer to backend/extensions_arch_test.go, and it
// carries more weight than that test does. On the Go side the compiler makes
// internal/** unreachable for free and the test holds the remainder; a bundler
// enforces nothing at all — it resolves whatever a path can reach — so on this
// side the boundary is exactly two things: frontend/package.json's `exports`
// map, and this file. Delete it and a unit can import the session store.
//
// Three rules, in any module file under an extension's frontend/ layer — ts,
// tsx, mts, cts, js, jsx, mjs, cjs, because a unit whose `main` is a .jsx is a
// unit this gate would otherwise never read:
//
//   1. A relative specifier escaping the unit's own frontend/ directory — the
//      deep import wearing a relative disguise.
//   2. A `@margince/frontend` subpath that is not in the exports map. The
//      allowed set is READ FROM the map rather than restated here, so widening
//      the surface is one edit in one place.
//   3. A bare specifier the unit's own package.json does not declare. A unit
//      that imports what it did not declare works only by accident of
//      hoisting, and breaks when another unit stops depending on it. A TEST
//      file may reach devDependencies; shipped code may not, because pulling a
//      test runner into the bundle is what that split exists to prevent.
//
// ## Why this is a parser and not a grep
//
// This replaces frontend/scripts/check-ext-imports.sh, whose specifier
// extraction was one regex over the whole file with its newlines collapsed to
// spaces, and which was candid about the two biases that forced on it: a quote
// CLASS rather than a matched pair, because ERE has no backreference, so `"x'`
// over-matched; and a collapsed line pairing a comment ending in `from` with
// the next line's string literal. Both were accepted because a parser was
// believed expensive. It is thirty lines, and neither bias is paid for now.
//
// The parser also SEES four spellings the regex could not, every one of which
// resolves at bundle time exactly like an import:
//
//   export { x } from "…"     a re-export is an import edge
//   export * from "…"         so is a star
//   require("…")              a unit whose main is a .cjs
//   import x = require("…")   the TypeScript form
//
// ## The one rule the parser has to be told to keep
//
// The shell gate did NOT strip comments, deliberately: a commented-out bad
// import is a bad import somebody is about to uncomment. A comment is not an
// AST node, so that rule would vanish SILENTLY on the port — the same shape as
// the .js coverage regression found reviewing the native-control gate's own
// port. Comment text is therefore scanned too, and a hit in one says so in the
// message rather than being reported as live code.
//
// The comments themselves come from the parser's trivia, not from a regex
// hunting `//`: a `//` inside a string is not a comment, and hand-written
// answers to that question are what this refactor exists to stop writing.

import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const frontendRoot = resolve(__dirname, "..");
const repoRoot = resolve(frontendRoot, "..");
const extensionsDir = join(repoRoot, "extensions");
const surfacePkg = join(frontendRoot, "package.json");

// Every extension a bundler resolves, not just the two a well-behaved unit
// writes: a `"main": "screen.jsx"` is a legal unit whose every file a
// TypeScript-only collector skips, which makes the whole gate opt-in.
const moduleFile = /\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$/;
const testFile = /\.test\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$/;

// A layer is any directory named `frontend` under extensions/, at ANY depth.
// The shell gate globbed `extensions/*/frontend`, so a unit that nests one was
// invisible to it — latent in today's tree, which has only top-level layers,
// and latent is exactly how a walk-shape hole survives to bite somebody later.
function extensionLayers(root: string): string[] {
  if (!existsSync(root)) return [];
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    if (!entry.isDirectory() || entry.name === "node_modules") return [];
    const full = join(root, entry.name);
    return entry.name === "frontend" ? [full] : extensionLayers(full);
  });
}

function moduleFilesUnder(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      // pnpm links the host into extensions/<unit>/frontend/node_modules/
      // @margince/frontend, so a walk that followed it would read the entire
      // core tree as if the unit shipped it.
      return entry.name === "node_modules" ? [] : moduleFilesUnder(full);
    }
    return moduleFile.test(entry.name) ? [full] : [];
  });
}

function readJson(path: string): Record<string, unknown> {
  return existsSync(path)
    ? (JSON.parse(readFileSync(path, "utf8")) as Record<string, unknown>)
    : {};
}

function keysOf(value: unknown): string[] {
  return typeof value === "object" && value !== null ? Object.keys(value) : [];
}

// The published subpaths, straight out of the map: "@margince/frontend/app"
// from "./app", and the bare root from ".". Read rather than restated here, so
// widening the surface is one edit in one place.
function publishedSubpaths(pkgPath: string): string[] {
  return keysOf(readJson(pkgPath).exports).map((key) =>
    key === "."
      ? "@margince/frontend"
      : `@margince/frontend/${key.replace(/^\.\//, "")}`,
  );
}

// A specifier, and how it was written — the second half is what lets a
// commented-out escape be reported as one rather than as live code.
type Specifier = { text: string; line: number; inComment: boolean };

function scriptKindFor(path: string): ts.ScriptKind {
  if (/\.(tsx|jsx)$/.test(path)) return ts.ScriptKind.TSX;
  if (/\.(js|mjs|cjs)$/.test(path)) return ts.ScriptKind.JS;
  return ts.ScriptKind.TS;
}

// Every comment in the file, taken from the PARSER's trivia. Each comment is
// the leading trivia of exactly one token — the end-of-file token carries the
// tail — so walking down to tokens reaches all of them, and `//` inside a
// string is not one of them for free.
function commentTexts(source: ts.SourceFile, text: string): Specifier[] {
  const out: Specifier[] = [];
  const seen = new Set<number>();
  const collect = (node: ts.Node): void => {
    for (const range of ts.getLeadingCommentRanges(text, node.getFullStart()) ??
      []) {
      if (seen.has(range.pos)) continue;
      seen.add(range.pos);
      const body = text.slice(range.pos, range.end);
      for (const m of body.matchAll(
        /(?:from|import|require)\s*\(?\s*["'`]([^"'`]+)["'`]/g,
      )) {
        const ahead = body.slice(0, m.index).split("\n").length - 1;
        out.push({
          text: m[1],
          line:
            source.getLineAndCharacterOfPosition(range.pos).line + 1 + ahead,
          inComment: true,
        });
      }
    }
    for (const child of node.getChildren(source)) collect(child);
  };
  collect(source);
  return out;
}

// specifiersIn returns every module specifier the file resolves, plus every one
// its comments spell out. Read directly by the probe suite below: a gate
// asserting a shape is ABSENT passes identically over a clean tree and over a
// detector that has stopped detecting.
function specifiersIn(path: string, text: string): Specifier[] {
  const source = ts.createSourceFile(
    path,
    text,
    ts.ScriptTarget.ES2022,
    true,
    scriptKindFor(path),
  );
  const out: Specifier[] = [];
  const push = (node: ts.Node, spec: string) =>
    out.push({
      text: spec,
      line:
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
      inComment: false,
    });

  const visit = (node: ts.Node): void => {
    // `import … from "x"`, and a bare `import "x"`.
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      push(node, node.moduleSpecifier.text);
    }
    // `export … from "x"` and `export * from "x"` — a re-export is an import
    // edge, and the regex this replaces saw it only because `from` happened to
    // appear in the same collapsed line.
    if (
      ts.isExportDeclaration(node) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      push(node, node.moduleSpecifier.text);
    }
    // `import x = require("y")`.
    if (
      ts.isImportEqualsDeclaration(node) &&
      ts.isExternalModuleReference(node.moduleReference) &&
      ts.isStringLiteral(node.moduleReference.expression)
    ) {
      push(node, node.moduleReference.expression.text);
    }
    // `import("x")` and `require("x")`.
    if (ts.isCallExpression(node)) {
      const dynamic = node.expression.kind === ts.SyntaxKind.ImportKeyword;
      const required =
        ts.isIdentifier(node.expression) && node.expression.text === "require";
      const [first] = node.arguments;
      if ((dynamic || required) && first && ts.isStringLiteral(first)) {
        push(node, first.text);
      }
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return [...out, ...commentTexts(source, text)];
}

// judge applies the three rules to one file's specifiers. `file` is absolute
// because rule 1 RESOLVES against it; `shown` is what a reader is handed. The
// two are separate parameters rather than one, because resolving a path that
// has already been made repo-relative silently resolves it against the process
// working directory instead, and every containment answer after that is a
// coincidence.
function judge(args: {
  file: string;
  shown: string;
  layer: string;
  specifiers: Specifier[];
  published: string[];
  declared: string[];
}): string[] {
  const { file, shown, layer, specifiers, published, declared } = args;
  const where = (s: Specifier) =>
    `${shown}:${s.line}${s.inComment ? " (commented out, and an import somebody is about to uncomment)" : ""}`;

  return specifiers.flatMap((s) => {
    if (s.text.startsWith(".")) {
      const resolved = resolve(dirname(file), s.text);
      // The layer itself, or something BENEATH it — the separator is
      // load-bearing. An unslashed prefix test accepts a sibling whose name
      // merely starts with the layer's: extensions/foo/frontend-lib is not
      // inside extensions/foo/frontend, and nothing scans it, because a layer
      // is a directory named exactly `frontend`.
      const inside = resolved === layer || resolved.startsWith(`${layer}/`);
      return inside
        ? []
        : [
            `${where(s)}: relative import '${s.text}' leaves the unit's own frontend/ — reach the core through @margince/frontend/<subpath>, never by path`,
          ];
    }
    if (s.text.startsWith("@margince/frontend")) {
      return published.includes(s.text)
        ? []
        : [
            `${where(s)}: '${s.text}' is not a published subpath — the surface is ${published.join(" ")}`,
          ];
    }
    // The package ROOT of "@scope/name/sub" or of "name/sub".
    const root = s.text.startsWith("@")
      ? s.text.split("/").slice(0, 2).join("/")
      : s.text.split("/")[0];
    return declared.includes(root)
      ? []
      : [
          `${where(s)}: '${s.text}' is not declared by the unit's frontend/package.json — a unit imports what it declares, or it works only by accident of hoisting (shipped code may not reach a devDependency)`,
        ];
  });
}

// auditLayers is the whole gate, parameterised by which tree it reads. The
// census below points it at the real extensions/; the fixture cases point it
// at a synthetic unit, because a gate proven only by "the tree is currently
// clean" is one that keeps passing after it stops working.
function auditLayers(roots: { extensions: string; surface: string }): {
  violations: string[];
  files: number;
  layers: string[];
} {
  const published = publishedSubpaths(roots.surface);
  const layers = extensionLayers(roots.extensions);
  // Shown relative to the parent of the tree being read, so a report names
  // `extensions/<unit>/frontend/…` whichever tree produced it — which is what
  // lets a fixture case assert the exact line a reader of the real census
  // would be handed.
  const shownRoot = dirname(roots.extensions);
  let files = 0;
  const violations = layers.flatMap((layer) => {
    const pkg = readJson(join(layer, "package.json"));
    // SHIPPED code may reach dependencies and peers. A screen importing a dev
    // dependency would pull a test runner into the bundle, so devDependencies
    // are deliberately absent from this set — and present in the other.
    const shipped = [
      ...keysOf(pkg.dependencies),
      ...keysOf(pkg.peerDependencies),
    ];
    const forTests = [...shipped, ...keysOf(pkg.devDependencies)];
    return moduleFilesUnder(layer).flatMap((file) => {
      files += 1;
      return judge({
        file,
        shown: relative(shownRoot, file),
        layer,
        specifiers: specifiersIn(file, readFileSync(file, "utf8")),
        published,
        declared: testFile.test(file) ? forTests : shipped,
      });
    });
  });
  return { violations, files, layers };
}

describe("a unit reaches the core only through the published surface", () => {
  it("finds no import escaping the unit, the exports map, or its own package.json", () => {
    expect(
      publishedSubpaths(surfacePkg).length,
      "frontend/package.json publishes no exports — the gate has no surface to hold a unit to",
    ).toBeGreaterThan(0);

    const { violations, layers } = auditLayers({
      extensions: extensionsDir,
      surface: surfacePkg,
    });
    // An empty scan means the gate is pointed at the wrong tree. A census that
    // judged nothing certifies nothing, and this one is fail-closed by design.
    expect(
      layers.length,
      "no extension frontend layer was found — this gate is dark",
    ).toBeGreaterThan(0);
    expect(violations).toEqual([]);
  });

  it("reaches every unit that has a frontend layer, at any depth", () => {
    // The walk's own test. The shell gate globbed extensions/*/frontend, so a
    // nested layer was unreachable — and nothing could notice, because the
    // real tree has none.
    const layers = extensionLayers(extensionsDir);
    for (const entry of readdirSync(extensionsDir, { withFileTypes: true })) {
      const layer = join(extensionsDir, entry.name, "frontend");
      if (!entry.isDirectory() || !existsSync(layer)) continue;
      expect(
        layers,
        `extensions/${entry.name}/frontend has a layer the walk did not reach`,
      ).toContain(layer);
    }
  });
});

// The half that makes the census above mean anything. Every case here is one
// the shell gate's own suite planted, plus the ones its regex could not have
// been asked.
describe("the extension-import detector sees what it claims to", () => {
  // A synthetic unit, because the real ones are supposed to pass.
  function scaffold(files: Record<string, string>, manifest?: string): string {
    const root = mkdtempSync(join(tmpdir(), "ext-imports-"));
    const layer = join(root, "extensions", "probe", "frontend");
    mkdirSync(layer, { recursive: true });
    writeFileSync(
      join(layer, "package.json"),
      manifest ??
        JSON.stringify({
          name: "@margince-ext/probe",
          private: true,
          main: "screen.tsx",
          peerDependencies: { react: "^19.0.0" },
          devDependencies: { vitest: "^3.0.0" },
        }),
    );
    for (const [name, body] of Object.entries(files)) {
      const full = join(layer, name);
      mkdirSync(dirname(full), { recursive: true });
      writeFileSync(full, body);
    }
    return root;
  }

  function audit(root: string): string[] {
    try {
      return auditLayers({
        extensions: join(root, "extensions"),
        surface: surfacePkg,
      }).violations;
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  }

  const ESCAPE = "../../../frontend/src/app/session";
  const cases: {
    name: string;
    want: string | null;
    files: Record<string, string>;
    manifest?: string;
    extra?: (root: string) => void;
  }[] = [
    // The deep import wearing a relative disguise — the whole reason this
    // exists — and the four other spellings of it. Each of these passed the
    // shell gate once: it is one thing to refuse the deep import a
    // well-behaved unit writes by accident, and another to refuse the one an
    // author is trying to sneak past.
    {
      name: "a relative escape into core",
      want: "leaves the unit's own frontend/",
      files: { "screen.tsx": `import { session } from "${ESCAPE}";` },
    },
    {
      name: "a relative escape written with single quotes",
      want: "leaves the unit's own frontend/",
      files: { "screen.tsx": `import { session } from '${ESCAPE}';` },
    },
    {
      name: "a relative escape split across lines",
      want: "leaves the unit's own frontend/",
      files: { "screen.tsx": `import { session } from\n  "${ESCAPE}";` },
    },
    // A unit's shipped code is not only its .tsx. A `main` of "screen.jsx" is
    // a fully unscanned unit if the collector only ever looks at TypeScript.
    {
      name: "a relative escape in a .js file the unit ships",
      want: "leaves the unit's own frontend/",
      files: {
        "screen.tsx": "export default function S() { return null }",
        "helper.js": `import { session } from "${ESCAPE}";`,
      },
    },
    // The containment test is a prefix test, so a sibling directory whose name
    // merely STARTS with the layer's reads as inside it unless the separator is
    // there — and nothing scans that sibling, because a layer is a directory
    // named exactly `frontend`.
    {
      name: "a relative escape into a sibling directory named like the layer",
      want: "leaves the unit's own frontend/",
      files: { "screen.tsx": 'import { steal } from "../frontend-lib/steal";' },
      extra: (root) => {
        const dir = join(root, "extensions", "probe", "frontend-lib");
        mkdirSync(dir, { recursive: true });
        writeFileSync(join(dir, "steal.ts"), "export const steal = 1;");
      },
    },
    {
      name: "an unpublished core subpath",
      want: "is not a published subpath",
      files: {
        "screen.tsx": 'import { thing } from "@margince/frontend/internals";',
      },
    },
    {
      name: "an undeclared npm package",
      want: "is not declared by",
      files: { "screen.tsx": 'import dayjs from "dayjs";' },
    },
    // A devDependency is for tests, and only for tests.
    {
      name: "shipped code importing a devDependency",
      want: "is not declared by",
      files: { "screen.tsx": 'import { it } from "vitest";' },
    },
    // The rule the parser had to be TOLD to keep: a comment is not an AST node,
    // so scanning only the tree would have dropped this silently.
    {
      name: "a commented-out escape",
      want: "commented out",
      files: {
        "screen.tsx": `// import { session } from "${ESCAPE}";\nexport default function S() { return null }`,
      },
    },
    // The spellings the collapsed-line regex could not see. Each resolves at
    // bundle time exactly like an import.
    {
      name: "a re-export of an undeclared package",
      want: "is not declared by",
      files: { "screen.tsx": 'export { x } from "dayjs";' },
    },
    {
      name: "an export-star escaping the unit",
      want: "leaves the unit's own frontend/",
      files: { "screen.tsx": `export * from "${ESCAPE}";` },
    },
    {
      name: "a require() escaping the unit",
      want: "leaves the unit's own frontend/",
      files: { "screen.cjs": `const s = require("${ESCAPE}");` },
    },
    {
      name: "an import-equals require escaping the unit",
      want: "leaves the unit's own frontend/",
      files: { "screen.ts": `import s = require("${ESCAPE}");` },
    },
    {
      name: "a dynamic import escaping the unit",
      want: "leaves the unit's own frontend/",
      files: { "screen.tsx": `const s = await import("${ESCAPE}");` },
    },
    // A scoped package's root is its first TWO segments. Taking only the first
    // reads "@tanstack" as the package and refuses a subpath the unit declared.
    {
      name: "an undeclared scoped package",
      want: "is not declared by",
      files: { "screen.tsx": 'import { x } from "@scope/thing/sub";' },
    },
    // And what it must ACCEPT. A gate that refuses everything holds nothing.
    {
      name: "a subpath of a declared scoped package",
      want: null,
      files: {
        "screen.tsx": 'import { q } from "@tanstack/react-query/build";',
      },
      manifest: JSON.stringify({
        name: "@margince-ext/probe",
        dependencies: { "@tanstack/react-query": "^5.0.0" },
      }),
    },
    {
      name: "a declared peer, the surface, and an internal relative import",
      want: null,
      files: {
        "screen.tsx":
          'import { useState } from "react";\nimport { Button } from "@margince/frontend/design-system";\nimport { helper } from "./helper";',
        "helper.ts": "export const helper = 1;",
      },
    },
    {
      name: "a test importing a declared devDependency",
      want: null,
      files: {
        "screen.tsx": "export default function S() { return null }",
        "screen.test.tsx": 'import { it } from "vitest";',
      },
    },
    {
      name: "a subpath of a declared package",
      want: null,
      files: { "screen.tsx": 'import { jsx } from "react/jsx-runtime";' },
    },
    // The bias the regex bought with its quote CLASS: a string that merely sits
    // next to the word `from` is not a specifier, and the parser knows it.
    {
      name: "a string that merely looks like a specifier",
      want: null,
      files: {
        "screen.tsx":
          'const s = "dayjs";\nconst t = ["from", "dayjs"];\nconst u = `import "dayjs"`;',
      },
    },
    // A `//` inside a string is not a comment — the residue the shell gate's
    // sibling had to answer by hand, and the parser answers for nothing.
    {
      name: "a URL in a string, whose // is not a comment",
      want: null,
      files: { "screen.tsx": 'const u = "https://h/a//b from \\"dayjs\\"";' },
    },
  ];

  for (const tc of cases) {
    it(`${tc.want === null ? "accepts" : "refuses"} ${tc.name}`, () => {
      const root = scaffold(tc.files, tc.manifest);
      tc.extra?.(root);
      const hits = audit(root);
      if (tc.want === null) {
        expect(hits).toEqual([]);
        return;
      }
      // The MESSAGE, not merely that something fired: three rules report on the
      // same file and a case that only counts hits cannot tell which one spoke.
      expect(hits.join("\n")).toContain(tc.want);
    });
  }

  it("names the line the reader has to open", () => {
    // A report that fires on the right file and names the wrong line sends its
    // reader hunting, and every line rule in the sibling gate's port survived
    // mutation because no case asserted one.
    const hits = audit(
      scaffold({
        "screen.tsx": `export const a = 1;\nimport { session } from "${ESCAPE}";`,
      }),
    );
    expect(hits).toEqual([
      `extensions/probe/frontend/screen.tsx:2: relative import '${ESCAPE}' leaves the unit's own frontend/ — reach the core through @margince/frontend/<subpath>, never by path`,
    ]);
  });

  it("refuses a unit whose layer is nested inside it", () => {
    // The walk shape the shell gate could not express. Nothing in the real tree
    // exercises it, which is why it needs a fixture rather than a census.
    const root = mkdtempSync(join(tmpdir(), "ext-imports-nested-"));
    const layer = join(root, "extensions", "probe", "panel", "frontend");
    mkdirSync(layer, { recursive: true });
    writeFileSync(
      join(layer, "package.json"),
      JSON.stringify({ name: "@margince-ext/probe" }),
    );
    writeFileSync(join(layer, "screen.tsx"), 'import dayjs from "dayjs";');
    expect(audit(root).join("\n")).toContain("is not declared by");
  });

  it("reads the surface out of the exports map, root included", () => {
    // The allowed set is DERIVED, so widening the surface is one edit in one
    // place — and the derivation has its own case because the real map has no
    // "." entry today, so nothing in the census exercises the root.
    const root = mkdtempSync(join(tmpdir(), "ext-imports-surface-"));
    const pkg = join(root, "package.json");
    writeFileSync(
      pkg,
      JSON.stringify({ exports: { ".": "./x.ts", "./app": "./y.ts" } }),
    );
    try {
      expect(publishedSubpaths(pkg)).toEqual([
        "@margince/frontend",
        "@margince/frontend/app",
      ]);
      // And a package with no map publishes nothing, which is what makes the
      // census's own fail-closed assertion mean something.
      writeFileSync(pkg, JSON.stringify({ name: "x" }));
      expect(publishedSubpaths(pkg)).toEqual([]);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("judges a layer that ships no package.json at all", () => {
    // The shell gate SKIPPED such a layer, so deleting a manifest was a way to
    // leave the unit ungated. A unit that declares nothing may import nothing,
    // which is the fail-closed reading and the only safe one.
    const root = mkdtempSync(join(tmpdir(), "ext-imports-bare-"));
    const layer = join(root, "extensions", "probe", "frontend");
    mkdirSync(layer, { recursive: true });
    writeFileSync(join(layer, "screen.tsx"), 'import dayjs from "dayjs";');
    expect(audit(root).join("\n")).toContain("is not declared by");
  });

  it("does not read a unit's node_modules", () => {
    // pnpm links the host into extensions/<unit>/frontend/node_modules, so a
    // walk that followed it would read the whole core tree as the unit's own
    // and report hundreds of escapes that are not the unit's doing.
    const root = scaffold({
      "screen.tsx": "export default function S() { return null }",
    });
    const nm = join(
      root,
      "extensions",
      "probe",
      "frontend",
      "node_modules",
      "dep",
    );
    mkdirSync(nm, { recursive: true });
    // An UNDECLARED bare import, not a relative escape: a relative specifier's
    // verdict depends on how deep the file sits, so from inside node_modules
    // the same `../../../` lands back INSIDE the layer and is legitimate. The
    // case would have passed with the guard removed, which is the shape of
    // fixture that proves nothing.
    writeFileSync(join(nm, "index.ts"), 'import dayjs from "dayjs";');
    expect(audit(root)).toEqual([]);
  });
});
