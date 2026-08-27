// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The extension frontend import gate: a unit's screen may reach the core ONLY
// through the published surface, and may reach npm only through what its own
// package declares.
//
// This is the frontend's answer to backend/gates/extensions_arch_test.go, and it
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
// answers to that question are what this refactor exists to stop writing. And
// each comment's BODY is parsed by the SAME extractor as the code, rather than
// matched for the words an import happens to contain — asking "is there a
// `from` near a quote" reports `// ported from '../legacy'` as an escape, and
// the fix for that is another guess about which prose is innocent.
//
// What that costs, stated here rather than discovered later: prose embedding a
// whole import STATEMENT is reported, because TypeScript's error recovery finds
// the statement inside it. Demanding the comment parse cleanly end to end would
// fix that and lose a doc comment quoting the old wiring under a line of prose
// — a miss, in the direction this gate must not be wrong in.

import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { extensionLayers, filesUnder, scriptKindFor } from "./lib/source-tree";

const frontendRoot = resolve(__dirname, "..");
const repoRoot = resolve(frontendRoot, "..");
const extensionsDir = join(repoRoot, "extensions");
const surfacePkg = join(frontendRoot, "package.json");

// A test file may reach devDependencies; shipped code may not. The extensions
// are the shared walk's set, spelled again here only because THIS half asks a
// different question of the same name — which of a unit's files is a test.
const testFile = /\.test\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$/;

function readJson(path: string): Record<string, unknown> {
  return existsSync(path)
    ? (JSON.parse(readFileSync(path, "utf8")) as Record<string, unknown>)
    : {};
}

// entriesOf reads a manifest section as name -> declared VALUE. The value is
// kept because rule 3 asks what the declaration resolves to, not only whether
// the name appears.
function entriesOf(value: unknown): [string, string][] {
  return typeof value === "object" && value !== null
    ? Object.entries(value).map(([k, v]) => [k, typeof v === "string" ? v : ""])
    : [];
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

// Every comment in the file, taken from the PARSER's trivia — so a `//` inside
// a string is not one of them, for free.
//
// A comment is trivia attached to a token, and there are two attachments, not
// one. This walked LEADING only, and the comment saying so claimed that reached
// them all; it did not, and the gap it hid was the commonest shape of the rule
// it was implementing. Both are read now, keyed by position so a comment
// attached from both sides is one comment.
function commentRanges(source: ts.SourceFile, text: string): ts.CommentRange[] {
  const out: ts.CommentRange[] = [];
  const seen = new Set<number>();
  const collect = (node: ts.Node): void => {
    // LEADING and TRAILING both, and the second is not a refinement.
    // `getLeadingCommentRanges` does not return a comment that sits on the same
    // line as the code before it, so `const a = 1; // import … from "../core"`
    // was invisible — which is the single most natural way to comment an import
    // out mid-edit, and the shell gate this replaces caught it because it read
    // text. Leading alone made the stated rule hold for a comment on its own
    // line and quietly not for one sharing a line.
    for (const range of [
      ...(ts.getLeadingCommentRanges(text, node.getFullStart()) ?? []),
      ...(ts.getTrailingCommentRanges(text, node.getEnd()) ?? []),
    ]) {
      if (seen.has(range.pos)) continue;
      seen.add(range.pos);
      out.push(range);
    }
    for (const child of node.getChildren(source)) collect(child);
  };
  collect(source);
  return out;
}

// unmarked blanks a comment's OPENING delimiter — `//` or `/*` — so what is
// left parses as source. It becomes a SPACE of the same width, never nothing,
// so every offset inside the comment stays exactly where the reader will find
// it.
//
// Only the opener, and that asymmetry is the interesting part. Leaving `/*` in
// place makes the whole body one unterminated comment and yields ZERO nodes —
// a silent miss, and a mutant removing it fails the suite. The CLOSING `*/`,
// and the `*` a block comment puts at the head of each line, are deliberately
// left alone: both looked equally necessary, both were written, and neither
// changes an answer — TypeScript's error recovery steps over a stray `*` and
// finds the statement behind it. The case below HOLDS that, rather than this
// paragraph asserting it: a claim about a measurement nobody can re-run is the
// kind this repo asks you to delete or gate.
//
// A line no test can fail without is a line the next reader has to reason about
// for nothing, so the rule is the one this suite applies to everything else:
// keep what a mutant kills.
function unmarked(raw: string): string {
  return raw.replace(/^\/\//, "  ").replace(/^\/\*/, "  ");
}

// specifiersIn returns every module specifier the file resolves, plus every one
// its comments spell out. Read directly by the probe suite below: a gate
// asserting a shape is ABSENT passes identically over a clean tree and over a
// detector that has stopped detecting.
function specifiersIn(path: string, text: string): Specifier[] {
  const source = parse(path, text);
  // A comment is not an AST node, and the shell gate deliberately did not strip
  // them: a commented-out bad import is a bad import somebody is about to
  // uncomment. Dropping that silently is the regression this file exists not to
  // repeat.
  //
  // Each comment's body is PARSED with the same extractor, rather than matched
  // for the words an import happens to contain. Asking "does this text contain
  // `from` near a quote" reports `// ported from '../legacy'` as an escape, and
  // then the fix is another guess about which prose is innocent. The question
  // the rule actually asks is whether the text IS an import statement, and the
  // parser is the thing that knows.
  const inComments = commentRanges(source, text).flatMap((range) => {
    const at = source.getLineAndCharacterOfPosition(range.pos).line;
    const body = parse(path, unmarked(text.slice(range.pos, range.end)));
    return astSpecifiers(body).map((s) => ({
      ...s,
      line: at + s.line,
      inComment: true,
    }));
  });
  return [...astSpecifiers(source), ...inComments];
}

// parse reads one source the one way this gate reads sources. A file is parsed
// ONCE and the same tree is handed to both the comment walk and the extractor:
// they each built their own, which is a second parse of identical text and,
// worse, two trees a later edit could make disagree.
function parse(path: string, text: string): ts.SourceFile {
  return ts.createSourceFile(
    path,
    text,
    ts.ScriptTarget.ES2022,
    true,
    scriptKindFor(path),
  );
}

// astSpecifiers is this gate's ONE reading of "what does this source import" —
// called twice, on the file and on each comment's body, because two readings of
// that question inside one gate are two answers that drift. Not the only such
// reading in the tree: scripts/test-budget.ts extracts specifiers too, for a
// different question, and merging them would be a worse answer to both.
function astSpecifiers(source: ts.SourceFile): Specifier[] {
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
    //
    // isStringLiteralLike, not isStringLiteral: a NoSubstitutionTemplateLiteral
    // is not a StringLiteral, and `import(`../../../frontend/src/app/session`)`
    // is a static edge a bundler resolves exactly like the quoted form. The
    // shell gate this replaces caught it — its quote class carried the backtick
    // — so requiring a StringLiteral here was a MISS the port introduced, in
    // the one direction this gate must never be wrong in.
    if (ts.isCallExpression(node)) {
      const dynamic = node.expression.kind === ts.SyntaxKind.ImportKeyword;
      const required =
        ts.isIdentifier(node.expression) && node.expression.text === "require";
      const [first] = node.arguments;
      if ((dynamic || required) && first && ts.isStringLiteralLike(first)) {
        push(node, first.text);
      }
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return out;
}

// throughLinks resolves a path the way a BUNDLER will, following any symlink on
// the way. Textual resolution alone is a way past rule 1: a unit that commits
//
//   extensions/evil/frontend/core -> ../../../frontend/src
//
// then writes `import "./core/app/session"`, and the specifier never textually
// leaves the layer. Vite follows the link (preserveSymlinks defaults false) and
// serves the core module. The shell gate this replaces missed it too, so it is
// not a regression — but it is a hole in the one thing holding this boundary.
//
// The target of an import usually does not exist as written — no extension, or
// a directory index — so the existing PREFIX is what gets resolved, and the
// unresolvable tail is put back. A path with no existing prefix at all comes
// back unchanged, which is the textual answer and the right fallback.
function throughLinks(target: string): string {
  const tail: string[] = [];
  let head = target;
  while (!existsSync(head)) {
    const parent = dirname(head);
    if (parent === head) return target;
    tail.unshift(basename(head));
    head = parent;
  }
  return join(realpathSync(head), ...tail);
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
  declared: Map<string, string>;
}): string[] {
  const { file, shown, layer, specifiers, published, declared } = args;
  const where = (s: Specifier) =>
    `${shown}:${s.line}${s.inComment ? " (commented out, and an import somebody is about to uncomment)" : ""}`;
  // Resolved ONCE. The layer does not change inside a judge call, and this is a
  // realpath syscall — inside the flatMap it ran per specifier, across every
  // import in every file of every unit.
  const realLayer = throughLinks(layer);

  return specifiers.flatMap((s) => {
    if (s.text.startsWith(".")) {
      const resolved = throughLinks(resolve(dirname(file), s.text));
      // The layer itself, or something BENEATH it — the separator is
      // load-bearing. An unslashed prefix test accepts a sibling whose name
      // merely starts with the layer's: extensions/foo/frontend-lib is not
      // inside extensions/foo/frontend, and nothing scans it, because a layer
      // is a directory named exactly `frontend`.
      const inside =
        resolved === realLayer || resolved.startsWith(`${realLayer}/`);
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
    const declaredAs = declared.get(root);
    if (declaredAs !== undefined) {
      // A name in `dependencies` is not the end of the question — the VALUE
      // decides what it resolves to. `"evil": "file:../../../frontend/src"` is
      // a declaration the gate used to accept, and pnpm resolves it straight
      // into core, so rule 3 was a check that a string appeared in a list.
      //
      // The refused set is the PATH protocols, and `workspace:` is not one of
      // them — both shipped units declare `"@margince/frontend": "workspace:*"`
      // today, so blocking it would refuse the tree as it stands. A workspace
      // descriptor names a declared member; `file:`, `link:` and yarn's
      // `portal:` name a directory, as does a bare `./` or `../` value, and a
      // directory is how a unit reaches something nobody published to it.
      return /^(file|link|portal):|^[./]/.test(declaredAs)
        ? [
            `${where(s)}: '${s.text}' is declared as '${declaredAs}', a path into the tree rather than a package — a unit reaches the core through @margince/frontend/<subpath>, never by resolving to it`,
          ]
        : [];
    }
    return [
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
  files: Map<string, number>;
  layers: string[];
} {
  const published = publishedSubpaths(roots.surface);
  const layers = extensionLayers(roots.extensions);
  // Shown relative to the parent of the tree being read, so a report names
  // `extensions/<unit>/frontend/…` whichever tree produced it — which is what
  // lets a fixture case assert the exact line a reader of the real census
  // would be handed.
  const shownRoot = dirname(roots.extensions);
  const files = new Map<string, number>();
  const violations = layers.flatMap((layer) => {
    const pkg = readJson(join(layer, "package.json"));
    // SHIPPED code may reach dependencies and peers. A screen importing a dev
    // dependency would pull a test runner into the bundle, so devDependencies
    // are deliberately absent from this set — and present in the other.
    const shipped = new Map<string, string>([
      ...entriesOf(pkg.dependencies),
      ...entriesOf(pkg.peerDependencies),
    ]);
    const forTests = new Map<string, string>([
      ...shipped,
      ...entriesOf(pkg.devDependencies),
    ]);
    return filesUnder(layer).flatMap((file) => {
      files.set(layer, (files.get(layer) ?? 0) + 1);
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

// layersThatReadNothing names the layers the walk found and read nothing
// inside. Such a layer judges nothing and reports the same word for it as a
// clean one: PASS.
//
// A function rather than an inline loop, because the assertion that uses it can
// only ever run against a tree where every layer HAS files — so a mutant in it
// survives, which is the census-of-zero problem one level up. Pulled out, the
// rule is reachable with a layer that read nothing, and the mutant dies.
//
// Per layer rather than a total, and that is the whole point: with three layers
// and ten files in one of them, `10 > 3` holds while two layers read nothing.
// A floor one populous layer can carry for the others is not a floor.
function layersThatReadNothing(
  layers: string[],
  files: Map<string, number>,
): string[] {
  return layers.filter((layer) => (files.get(layer) ?? 0) === 0);
}

describe("a unit reaches the core only through the published surface", () => {
  it("finds no import escaping the unit, the exports map, or its own package.json", () => {
    expect(
      publishedSubpaths(surfacePkg).length,
      "frontend/package.json publishes no exports — the gate has no surface to hold a unit to",
    ).toBeGreaterThan(0);

    const { violations, layers, files } = auditLayers({
      extensions: extensionsDir,
      surface: surfacePkg,
    });
    // An empty scan means the gate is pointed at the wrong tree. A census that
    // judged nothing certifies nothing, and this one is fail-closed by design.
    expect(
      layers.length,
      "no extension frontend layer was found — this gate is dark",
    ).toBeGreaterThan(0);
    // And a FILE floor, PER LAYER — see layersThatReadNothing.
    expect(
      layersThatReadNothing(layers, files).map((l) => relative(repoRoot, l)),
      "these layers were found but no file inside them was read",
    ).toEqual([]);
    expect(violations).toEqual([]);
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
    // Prose is not an import. Matching the WORDS an import contains reported
    // every one of these, and the fix for that is not a better word list — it
    // is asking the parser whether the text is an import statement at all.
    {
      name: "a comment saying where something was ported from",
      want: null,
      files: {
        "screen.tsx":
          "// ported from '../../../frontend/src/app/session'\nexport default function S() { return null }",
      },
    },
    {
      name: "a comment naming a package in prose",
      want: null,
      files: {
        "screen.tsx":
          '// we read the date from "dayjs" here once, and stopped\nexport default function S() { return null }',
      },
    },
    // A SYMLINK inside the layer: the specifier never textually leaves, and a
    // bundler follows the link straight into core.
    {
      name: "a relative import through a symlink pointing at core",
      want: "leaves the unit's own frontend/",
      files: { "screen.tsx": 'import { session } from "./core/app/session";' },
      extra: (root) => {
        const layer = join(root, "extensions", "probe", "frontend");
        const core = join(root, "core-src");
        mkdirSync(join(core, "app"), { recursive: true });
        writeFileSync(join(core, "app", "session.ts"), "export const s = 1;");
        symlinkSync(core, join(layer, "core"), "dir");
      },
    },
    // A file in a SUBDIRECTORY of the layer: rule 1 resolves against the
    // importing FILE, not against the layer, and nothing here could tell the
    // two apart while every fixture sat at the layer root.
    {
      name: "a nested file whose escape is shallower than the layer root's",
      want: "leaves the unit's own frontend/",
      files: {
        "screen.tsx": "export default function S() { return null }",
        "panels/inner.tsx":
          'import { session } from "../../../../frontend/src/app/session";',
      },
    },
    {
      name: "a nested file reaching back up inside the layer",
      want: null,
      files: {
        "screen.tsx": "export default function S() { return null }",
        "helper.ts": "export const helper = 1;",
        "panels/inner.tsx": 'import { helper } from "../helper";',
      },
    },
    // The layer directory ITSELF is inside the layer — the `resolved === layer`
    // half of the containment test, which nothing reached.
    {
      name: "an import of the layer directory itself",
      want: null,
      files: { "screen.tsx": 'import { x } from "../frontend";' },
    },
    // Rule 3 asked whether a NAME appears in the manifest. pnpm resolves the
    // VALUE, and a path protocol resolves wherever it points.
    {
      name: "a dependency declared as a path into the tree",
      want: "a path into the tree rather than a package",
      files: { "screen.tsx": 'import { session } from "evil/app/session";' },
      manifest: JSON.stringify({
        name: "@margince-ext/probe",
        dependencies: { evil: "file:../../../frontend/src" },
      }),
    },
    {
      name: "a dependency declared as a link protocol",
      want: "a path into the tree rather than a package",
      files: { "screen.tsx": 'import { x } from "sneaky";' },
      manifest: JSON.stringify({
        name: "@margince-ext/probe",
        dependencies: { sneaky: "link:../../../frontend/src" },
      }),
    },
    {
      name: "a dependency declared as a yarn portal",
      want: "a path into the tree rather than a package",
      files: { "screen.tsx": 'import { x } from "sneaky";' },
      manifest: JSON.stringify({
        name: "@margince-ext/probe",
        dependencies: { sneaky: "portal:../../../frontend/src" },
      }),
    },
    {
      name: "a dependency declared as a bare relative path",
      want: "a path into the tree rather than a package",
      files: { "screen.tsx": 'import { x } from "sneaky";' },
      manifest: JSON.stringify({
        name: "@margince-ext/probe",
        dependencies: { sneaky: "../../../frontend/src" },
      }),
    },
    // `workspace:` is NOT a path protocol, and refusing it would refuse the
    // tree as it stands: both shipped units declare the surface that way.
    {
      name: "a workspace descriptor, which both real units use",
      want: null,
      files: { "screen.tsx": 'import { q } from "@tanstack/react-query";' },
      manifest: JSON.stringify({
        name: "@margince-ext/probe",
        peerDependencies: { "@tanstack/react-query": "workspace:*" },
      }),
    },
    {
      name: "a dependency declared as an ordinary version range",
      want: null,
      files: { "screen.tsx": 'import { useState } from "react";' },
    },
    // A TEMPLATE literal is a specifier. The port required a StringLiteral and
    // a NoSubstitutionTemplateLiteral is not one, so these two shipped past a
    // gate the shell script had caught — its quote class carried the backtick.
    {
      name: "a dynamic import whose specifier is a template literal",
      want: "leaves the unit's own frontend/",
      files: { "screen.tsx": `const s = await import(\`${ESCAPE}\`);` },
    },
    {
      name: "a require whose specifier is a template literal",
      want: "leaves the unit's own frontend/",
      files: { "screen.cjs": `const s = require(\`${ESCAPE}\`);` },
    },
    // A comment sharing a line with code is a comment. Leading trivia does not
    // carry one, so the rule held for a comment on its own line and quietly did
    // not for the commonest way to comment an import out mid-edit.
    {
      name: "an escape commented out at the end of a line of code",
      want: "commented out",
      files: {
        "screen.tsx": `const a = 1; // import { session } from "${ESCAPE}";`,
      },
    },
    // The parser-trivia design, held rather than asserted: a `//` inside a
    // STRING is not a comment. The previous case for this could not fail —
    // its string's tail held no import keyword, so a regex-based comment
    // finder passed it too.
    {
      name: "a string whose // tail is itself an import statement",
      want: null,
      files: {
        "screen.tsx":
          "const doc = 'see // import dayjs from \"dayjs\"';\nexport default function S() { return null }",
      },
    },
    // And the line this rule DOES draw, stated rather than discovered: prose
    // that embeds a whole import STATEMENT is reported. `drop import "dayjs"
    // when the shim goes` does not parse, but TypeScript's error recovery finds
    // the real `import "dayjs"` inside it, and that is the honest answer — the
    // text is an import statement with words around it.
    {
      name: "prose that embeds a whole import statement",
      want: "is not declared by",
      files: {
        "screen.tsx":
          '/* drop import "dayjs" when the shim goes */\nexport default function S() { return null }',
      },
    },
    // A doc comment's leading `*` must not stop the body parsing, or the rule
    // above holds for `//` and quietly does not for `/** */`.
    {
      name: "a doc comment holding a real commented-out escape",
      want: "commented out",
      files: {
        "screen.tsx":
          '/**\n * import { session } from "../../../frontend/src/app/session";\n */\nexport default function S() { return null }',
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

  it("needs no trailing delimiter or line-head star blanked", () => {
    // The absence two lines were deleted for. Each shape is read twice — as the
    // gate reads it, and with `*/` and the line-head `*` also blanked — and the
    // two must agree, or one of those lines was load-bearing after all.
    //
    // An absence is the hardest thing to keep true: nothing fails when somebody
    // re-adds a strip "for safety", and the comment explaining why it is not
    // there stops matching the code silently.
    const alsoBlanked = (raw: string) =>
      raw
        .replace(/^\/\//, "  ")
        .replace(/^\/\*/, "  ")
        .replace(/\*\/$/, "  ")
        .replace(/^(\s*)\*/gm, "$1 ");
    const shapes = [
      '/**\n * import { s } from "../x";\n */',
      '/**\n * export * from "../x";\n */',
      '/**\n * export { s } from "../x";\n */',
      '/**\n * import s = require("../x");\n */',
      '/**\n * const s = require("../x");\n */',
      '/**\n * const s = await import("../x");\n */',
      '/**\n * import "../x";\n */',
      '/**\n * import a from "../x";\n * import b from "../y";\n */',
      '/**\n *import { s } from "../x";\n */',
      '/* import { s } from "../x"; */',
      '/* const s = require("../x")*/',
    ];
    for (const raw of shapes) {
      const asRead = astSpecifiers(parse("c.ts", unmarked(raw))).map(
        (x) => x.text,
      );
      const asIfBlanked = astSpecifiers(parse("c.ts", alsoBlanked(raw))).map(
        (x) => x.text,
      );
      expect(asRead, `blanking more changed the answer for: ${raw}`).toEqual(
        asIfBlanked,
      );
      expect(asRead.length, `nothing found in: ${raw}`).toBeGreaterThan(0);
    }
  });

  it("names the line inside a multi-line comment, not the comment's first", () => {
    // A comment's body is parsed on its own, so its line numbers start at 1 and
    // have to be shifted back onto the file. Without the shift every hit in a
    // block comment names the line the comment OPENED on.
    //
    // The two lines of code above the comment are the whole point: with the
    // comment opening on line 1 the shift adds zero, and this case passed
    // identically with the shift removed. A fixture that cannot fail on the
    // rule it is named for is the defect this suite exists to stop shipping.
    const hits = audit(
      scaffold({
        "screen.tsx":
          'export const a = 1;\nexport const b = 2;\n/*\n * a note\n * import { session } from "../../../frontend/src/app/session";\n */\nexport default function S() { return null }',
      }),
    );
    expect(hits).toEqual([
      `extensions/probe/frontend/screen.tsx:5 (commented out, and an import somebody is about to uncomment): relative import '${ESCAPE}' leaves the unit's own frontend/ — reach the core through @margince/frontend/<subpath>, never by path`,
    ]);
  });

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

  it("refuses a layer that read no file, even beside one that read many", () => {
    // The floor is per LAYER because a total is satisfied by the wrong thing:
    // one populous layer carries the count for a layer that read nothing, and
    // a layer judged on zero files reports the same word as a clean one, PASS.
    const root = mkdtempSync(join(tmpdir(), "ext-imports-floor-"));
    try {
      const full = join(root, "extensions", "full", "frontend");
      const empty = join(root, "extensions", "empty", "frontend");
      mkdirSync(full, { recursive: true });
      mkdirSync(empty, { recursive: true });
      for (const dir of [full, empty]) {
        writeFileSync(join(dir, "package.json"), JSON.stringify({ name: "p" }));
      }
      writeFileSync(join(full, "a.tsx"), "export const a = 1;");
      writeFileSync(join(full, "b.tsx"), "export const b = 2;");
      const { files, layers } = auditLayers({
        extensions: join(root, "extensions"),
        surface: surfacePkg,
      });
      expect(layers.length).toBe(2);
      // The TOTAL clears any sane floor — two files against two layers — while
      // one of the two read nothing. That is the assertion shape being refused.
      expect([...files.values()].reduce((a, b) => a + b, 0)).toBe(2);
      expect(layersThatReadNothing(layers, files)).toEqual([empty]);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
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
