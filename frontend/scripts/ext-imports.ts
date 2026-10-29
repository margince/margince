// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The extension import detector that ext-imports.test.ts runs over the tree
// and over its own fixtures. The rules it holds are written in that file.

import { existsSync, readFileSync, realpathSync } from "node:fs";
import { basename, dirname, join, relative, resolve } from "node:path";
import ts from "typescript";
import { extensionLayers, filesUnder, parseSource } from "./lib/source-tree";

export const frontendRoot = resolve(__dirname, "..");
export const repoRoot = resolve(frontendRoot, "..");
export const extensionsDir = join(repoRoot, "extensions");
export const surfacePkg = join(frontendRoot, "package.json");

// A test file may reach devDependencies; shipped code may not. The extensions
// repeat the shared walk's set, because this half asks a different question:
// which of a unit's files is a test.
export const testFile = /\.test\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$/;

export function readJson(path: string): Record<string, unknown> {
  return existsSync(path)
    ? (JSON.parse(readFileSync(path, "utf8")) as Record<string, unknown>)
    : {};
}

// entriesOf reads a manifest section as name -> declared VALUE. The value is
// kept because rule 3 asks what the declaration resolves to, not only whether
// the name appears.
export function entriesOf(value: unknown): [string, string][] {
  return typeof value === "object" && value !== null
    ? Object.entries(value).map(([k, v]) => [k, typeof v === "string" ? v : ""])
    : [];
}

export function keysOf(value: unknown): string[] {
  return typeof value === "object" && value !== null ? Object.keys(value) : [];
}

// The published subpaths, straight out of the map: "@margince/frontend/app"
// from "./app", and the bare root from ".". Read rather than restated here, so
// widening the surface is one edit in one place.
export function publishedSubpaths(pkgPath: string): string[] {
  return keysOf(readJson(pkgPath).exports).map((key) =>
    key === "."
      ? "@margince/frontend"
      : `@margince/frontend/${key.replace(/^\.\//, "")}`,
  );
}

// A specifier, and how it was written. The second half lets a commented-out
// escape be reported as one rather than as live code.
export type Specifier = { text: string; line: number; inComment: boolean };

// Every comment in the file, taken from the parser's trivia, so a `//` inside a
// string is not one of them.
//
// A comment is trivia attached to a token, as leading or trailing trivia. Both
// are read, keyed by position, so a comment attached from both sides counts once.
export function commentRanges(
  source: ts.SourceFile,
  text: string,
): ts.CommentRange[] {
  const out: ts.CommentRange[] = [];
  const seen = new Set<number>();
  const collect = (node: ts.Node): void => {
    // `getLeadingCommentRanges` skips a comment on the same line as the code
    // before it, such as `const a = 1; // import … from "../core"`. That is
    // the commonest way to comment an import out, so trailing ranges count too.
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

// unmarked blanks a comment's opening delimiter (`//` or `/*`) so what is left
// parses as source. It becomes spaces of the same width, so every offset inside
// the comment stays where the reader will find it.
//
// Only the opener. Leaving `/*` in place makes the body one unterminated comment
// with zero nodes, a silent miss, and a mutant removing it fails the suite.
// The closing `*/` and the `*` at the head of each block line are left alone.
// TypeScript's error recovery steps over a stray `*` and finds the statement
// behind it, and a test case holds that. Code no mutant can kill is not kept.
export function unmarked(raw: string): string {
  return raw.replace(/^\/\//, "  ").replace(/^\/\*/, "  ");
}

// specifiersIn returns every module specifier the file resolves, plus every one
// its comments spell out. The probe suite reads it directly. A gate asserting a
// shape is absent passes the same over a clean tree and a broken detector.
export function specifiersIn(path: string, text: string): Specifier[] {
  const source = parse(path, text);
  // A comment is not an AST node, but comments are read: a commented-out bad
  // import is a bad import somebody is about to uncomment.
  //
  // Each comment's body is parsed with the same extractor, not matched for the
  // words an import contains. A word match reports `// ported from '../legacy'`
  // as an escape. The rule asks whether the text is an import statement, and
  // the parser knows that.
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
// once, and the comment walk and the extractor share that tree. Two trees of
// one text could be made to disagree by a later edit.
export function parse(path: string, text: string): ts.SourceFile {
  return parseSource(path, text);
}

// astSpecifiers is this gate's single reading of "what does this source import".
// It runs on the file and on each comment's body, so one gate has one answer.
// scripts/test-budget.consts.ts extracts specifiers too, for a different
// question, and merging them would answer both worse.
export function astSpecifiers(source: ts.SourceFile): Specifier[] {
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
    // `export … from "x"` and `export * from "x"`: a re-export is an import
    // edge too.
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
    // is not a StringLiteral. Yet `import(`../../../frontend/src/app/session`)`
    // is a static edge a bundler resolves like the quoted form, so missing it
    // would let an escape pass.
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

// throughLinks resolves a path the way a bundler will, following any symlink.
// Textual resolution alone is a way past rule 1. A unit commits
// `extensions/evil/frontend/core -> ../../../frontend/src`, then writes
// `import "./core/app/session"`. Vite follows the link (preserveSymlinks
// defaults false) and serves core.
//
// An import target often has no extension or names a directory index. So the
// existing prefix is resolved and the tail put back; with no prefix, the path
// comes back unchanged.
export function throughLinks(target: string): string {
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

const CORE_ORIGINAL = "@core-original/";
const OVERRIDES = "overrides";

const stripExtension = (path: string) => path.replace(/\.[cm]?[jt]sx?$/, "");

// replacedOriginal is the one `@core-original/` specifier a replacement under
// frontend/overrides/ may use: core's copy of the file it stands in for. An
// added screen under src/custom/ replaces nothing, so it has none.
function replacedOriginal(file: string, realLayer: string): string | undefined {
  const target = relative(join(realLayer, OVERRIDES), throughLinks(file));
  if (target.startsWith("..") || target.startsWith("src/custom/")) {
    return undefined;
  }
  return CORE_ORIGINAL + stripExtension(target);
}

function judgeCoreOriginal(
  s: Specifier,
  original: string | undefined,
  where: (s: Specifier) => string,
): string[] {
  return original !== undefined && stripExtension(s.text) === original
    ? []
    : [
        `${where(s)}: '${s.text}' reaches core's own files — only a replacement under frontend/overrides/ may, and only the one file it replaces`,
      ];
}

// judge applies the three rules to one file's specifiers. `file` is absolute
// because rule 1 resolves against it; `shown` is what a reader is handed. A
// repo-relative path would resolve against the working directory instead, and
// every containment answer would be wrong.
export function judge(args: {
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
  // Resolved once: the layer does not change inside a judge call, and this is a
  // realpath syscall that would otherwise run per specifier.
  const realLayer = throughLinks(layer);

  const original = replacedOriginal(file, realLayer);

  return specifiers.flatMap((s) => {
    if (s.text.startsWith(CORE_ORIGINAL)) {
      return judgeCoreOriginal(s, original, where);
    }
    if (s.text.startsWith(".")) {
      const resolved = throughLinks(resolve(dirname(file), s.text));
      // The layer itself, or something beneath it, so the separator matters.
      // Without it, extensions/foo/frontend-lib would pass as inside
      // extensions/foo/frontend. Nothing scans it, because a layer is a
      // directory named `frontend`.
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
      // A name in `dependencies` is not enough: the value decides what it
      // resolves to. pnpm resolves `"evil": "file:../../../frontend/src"`
      // straight into core.
      //
      // The refused set is the path protocols. `workspace:` names a declared
      // member, and both shipped units declare `"@margince/frontend":
      // "workspace:*"`. `file:`, `link:`, yarn's `portal:` and a bare `./` or
      // `../` name a directory, which reaches what nobody published.
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
// census points it at the real extensions/, and fixture cases at a synthetic
// unit. A gate proven only on a clean tree keeps passing after it breaks.
export function auditLayers(roots: { extensions: string; surface: string }): {
  violations: string[];
  files: Map<string, number>;
  layers: string[];
} {
  const published = publishedSubpaths(roots.surface);
  const layers = extensionLayers(roots.extensions);
  // Shown relative to the parent of the tree being read, so a report names
  // `extensions/<unit>/frontend/…` whichever tree produced it. A fixture case
  // can then assert the line the real census would print.
  const shownRoot = dirname(roots.extensions);
  const files = new Map<string, number>();
  const violations = layers.flatMap((layer) => {
    const pkg = readJson(join(layer, "package.json"));
    // Shipped code may reach dependencies and peers. A screen importing a dev
    // dependency would pull a test runner into the bundle, so devDependencies
    // are in the test set only.
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
// A function, not an inline loop: the real tree has files in every layer, so a
// mutant inline would survive. Pulled out, a test reaches an empty layer.
//
// Per layer, not a total. With three layers and ten files in one, `10 > 3`
// holds while two layers read nothing.
export function layersThatReadNothing(
  layers: string[],
  files: Map<string, number>,
): string[] {
  return layers.filter((layer) => (files.get(layer) ?? 0) === 0);
}
