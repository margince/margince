// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Mono is for code, and only for code.
//
// The code face is Geist Mono, worn only by `code`, `pre`, `samp` and
// `.code-block`. An id, a key, a timestamp and an amount read in the body
// face; a figure that lines up in a column gets `tabular-nums` (`.t-num`).
//
// Three spellings bring it back, and each has an arm:
//  (a) the `t-mono` class, in markup, a stylesheet or a selector string;
//  (b) a `font-family` or `font` naming a mono family on a rule whose
//      selectors do not all target code, plus any custom property carrying
//      one except the `--fontFamilyMono` token in tokens.css;
//  (c) the same family inside a TypeScript string.
//
// scripts/check-font-lock.sh carries the same three arms as a grep.

import ts from "typescript";
import {
  isBalanced,
  resolveNesting,
  splitTopLevel,
  subjectOf,
} from "../../scripts/lib/css-rules";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

// `monospace` also matches inside `ui-monospace`, which is intended: a hyphen is
// a word boundary.
const MONO_FAMILY = /var\(\s*--fontFamilyMono\s*\)|Geist Mono|\bmonospace\b/i;
const T_MONO = /(?<![\w-])t-mono(?![\w-])/;

// A string that only looks like CSS, `for (…; i++) {`, opens no selector.
function selectorsInside(parents: string[], prelude: string): string[] {
  if (prelude.startsWith("@")) return parents;
  return isBalanced(prelude) ? resolveNesting(parents, prelude) : [];
}

/**
 * Whether a selector's subject, the element the declarations land on, is
 * code. `.foo code` is; `code .foo` is not, because the rule dresses `.foo`.
 */
function targetsCode(selector: string): boolean {
  const compound = splitTopLevel(subjectOf(selector), ":")[0] ?? "";
  return (
    /^(code|pre|samp)(?![\w-])/i.test(compound) ||
    /\.code-block(?![\w-])/.test(compound)
  );
}

type MonoDeclaration = Readonly<{
  line: number;
  property: string;
  selectors: readonly string[];
}>;

/**
 * Every declaration in a stylesheet that names a mono family, with the
 * selectors it applies to. A declaration outside any rule (a `style`
 * attribute's contents) applies to no selector, so it can never target code.
 */
export function monoDeclarations(css: string): MonoDeclaration[] {
  const text = css.replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    comment.replace(/[^\n]/g, " "),
  );
  const found: MonoDeclaration[] = [];
  // The selectors each open block applies to. An at-rule block (`@media`,
  // `@supports`) applies to whatever its parent did.
  const open: string[][] = [];
  let start = 0;
  const judge = (end: number) => {
    const chunk = text.slice(start, end);
    const match = /^\s*(font-family|font|--[\w-]+)\s*:([\s\S]*)$/i.exec(chunk);
    if (match && MONO_FAMILY.test(match[2])) {
      const offset = start + chunk.search(/\S/);
      found.push({
        line: text.slice(0, offset).split("\n").length,
        property: match[1],
        selectors: open.at(-1) ?? [],
      });
    }
  };
  for (let index = 0; index < text.length; index++) {
    const char = text[index];
    if (char === "{") {
      const prelude = text.slice(start, index).trim();
      const parents = open.at(-1) ?? [];
      open.push(selectorsInside(parents, prelude));
      start = index + 1;
    } else if (char === ";" || char === "}") {
      judge(index);
      if (char === "}") open.pop();
      start = index + 1;
    }
  }
  judge(text.length);
  return found;
}

export function isAllowed(file: string, declaration: MonoDeclaration): boolean {
  if (declaration.property.startsWith("--")) {
    return (
      declaration.property === "--fontFamilyMono" &&
      file.endsWith("design-system/tokens.css")
    );
  }
  return (
    declaration.selectors.length > 0 && declaration.selectors.every(targetsCode)
  );
}

function stylesheetViolations(
  file: string,
  css: string,
  lineOffset: number,
): string[] {
  const where = (line: number) => `${file}:${line + lineOffset}`;
  const blanked = css.replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    comment.replace(/[^\n]/g, " "),
  );
  const classHits = blanked
    .split("\n")
    .flatMap((line, index) =>
      T_MONO.test(line)
        ? [
            `${where(index + 1)} names the t-mono class — use t-num for a figure`,
          ]
        : [],
    );
  const familyHits = monoDeclarations(css)
    .filter((declaration) => !isAllowed(file, declaration))
    .map(
      ({ line, property, selectors }) =>
        `${where(line)} ${property} names a mono family on ${
          selectors.length > 0 ? `"${selectors.join(", ")}"` : "no selector"
        } — mono is for code, pre, samp and .code-block only`,
    );
  return [...classHits, ...familyHits];
}

function htmlViolations(file: string, html: string): string[] {
  const text = html.replace(/<!--[\s\S]*?-->/g, (comment) =>
    comment.replace(/[^\n]/g, " "),
  );
  const lineAt = (offset: number) => text.slice(0, offset).split("\n").length;
  return [
    ...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/gi),
    ...text.matchAll(/\bstyle\s*=\s*"([^"]*)"/gi),
    ...text.matchAll(/\bclass\s*=\s*"([^"]*)"/gi),
  ].flatMap((match) =>
    stylesheetViolations(file, match[1], lineAt(match.index ?? 0) - 1),
  );
}

// Strings only. A comment discussing the rule is not breaking it. JSX text
// is words on the screen, which cannot carry a class or set a face.
function scriptViolations(file: string, source: string): string[] {
  const parsed = parseSource(file, source);
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isStringLiteral(node) ||
      ts.isNoSubstitutionTemplateLiteral(node) ||
      ts.isTemplateHead(node) ||
      ts.isTemplateMiddle(node) ||
      ts.isTemplateTail(node)
    ) {
      const line =
        parsed.getLineAndCharacterOfPosition(node.getStart(parsed)).line + 1;
      if (T_MONO.test(node.text)) {
        found.push(
          `${file}:${line} names the t-mono class — use t-num for a figure`,
        );
      }
      // A string naming the family is one of two things. It is a stylesheet in
      // a string, judged rule by rule. Or it is a bare value (an inline
      // `fontFamily`, a constant) that no rule scopes to code at all.
      const declarations = monoDeclarations(node.text);
      const bare = declarations.length === 0 && MONO_FAMILY.test(node.text);
      if (bare || declarations.some((entry) => !isAllowed(file, entry))) {
        found.push(
          `${file}:${line} names a mono family in a string — mono is for code, pre, samp and .code-block only`,
        );
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(parsed);
  return found;
}

/** Every reintroduction of mono outside code in one source text. */
export function monoViolations(file: string, text: string): string[] {
  if (file.endsWith(".css")) return stylesheetViolations(file, text, 0);
  if (file.endsWith(".html")) return htmlViolations(file, text);
  return scriptViolations(file, text);
}

// Every source a bundler, Storybook or Playwright reads. A test naming the
// class or the family is the assertion, and a `.d.ts` can set no face.
const SOURCE = /\.(css|html|[cm]?[jt]sx?)$/;
const NOT_SOURCE = /\.test\.[cm]?[jt]sx?$|\.d\.ts$/;

const isSource = (file: CensusFile) =>
  (file.area === "root"
    ? file.rel === "index.html"
    : file.area !== "scripts") &&
  SOURCE.test(file.path) &&
  !NOT_SOURCE.test(file.path);

const REACHED = [
  "src/design-system/base.css",
  "src/design-system/tokens.css",
  "src/mcp-apps/view.css",
  "index.html",
];

export const rule: CensusRule = {
  name: "mono",
  corpus: (tree) => tree.files.filter(isSource),
  // The base rule proves the parser reaches a real declaration: a parser that
  // read nothing would report this tree clean for free.
  floors: (corpus) => {
    const names = new Set(corpus.map((file) => file.rel));
    const base = corpus.find(
      (file) => file.rel === "src/design-system/base.css",
    );
    const codeRule = monoDeclarations(base?.text ?? "");
    return [
      ...REACHED.filter((name) => !names.has(name)).map(
        (name) => `the census did not read ${name}`,
      ),
      ...(codeRule.length > 0 &&
      codeRule.every((entry) => isAllowed("base.css", entry))
        ? []
        : ["base.css sets no code face this rule can read"]),
      ...(corpus.some((file) => file.area === "e2e")
        ? []
        : ["the census reached no e2e module"]),
      ...(corpus.some((file) => file.area === "storybook")
        ? []
        : ["the census reached no Storybook config"]),
    ];
  },
  visit: (file) => monoViolations(file.rel, file.text),
  advice:
    "Mono is for code, pre, samp and .code-block only; a figure that must line up uses t-num.",
};
