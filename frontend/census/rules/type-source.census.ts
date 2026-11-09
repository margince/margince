// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One source for type. Outside tokens.css nothing under src/ declares a size,
// a leading, a weight or a tracking by value. A rule wears a `--font*` token or
// it inherits.
//
// A hand-typed size renders and fails nothing. When the token moves, the copy
// does not, and two screens drift a hair apart. A weight the request never
// loaded is synthesized by the engine (weights.census.ts).

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

// The one file that may declare type by value.
export const SOURCE = "src/design-system/tokens.css";

// The two UA resets. Each may hand a control back its platform casing
// (`text-transform: none`), and set `line-height: 0` on `sub`/`sup` alone so a
// footnote ordinal does not grow its line. Everywhere else both are restyling.
export const RESETS: readonly string[] = [
  "src/app.css",
  "src/mcp-apps/view.css",
];

// The selector list a `line-height: 0` may stand on: `sub` and `sup` only.
const NEUTRALISED = /^(sub|sup)(\s*,\s*(sub|sup))*$/;

// A token reference: `--font` plus a capital, so `--fontBody` and
// `--fontWeightMedium` match and `--font-size-13` does not.
const TYPE_TOKEN = /^var\(\s*--font[A-Z][A-Za-z]*\s*\)$/;
const WEIGHT_TOKEN = /^var\(\s*--fontWeight[A-Z][A-Za-z]*\s*\)$/;

const INHERIT = "inherit";

// Longest property first, so `font-size` is tried before `font`. `font-family`
// is the face gate's question. `font-variant-numeric` is left out, which allows
// the tabular figures `.t-num` draws.
const CSS_DECLARATION =
  /(?:^|[;{}\s])(font-feature-settings|font-variant-caps|font-variant|font-size|font-weight|line-height|letter-spacing|text-transform|font)\s*:\s*([^;}]*)/g;

// Small caps draw capitals by another road, so they answer to the same rule
// as `text-transform`. smcp is small caps, c2sc caps-to-small-caps, pcap petite.
const CAPS_KEYWORD =
  /\b(small-caps|all-small-caps|petite-caps|all-petite-caps|unicase|titling-caps)\b/;
const CAPS_FEATURE = /["'](smcp|c2sc|pcap)["']/;

// A style object's property, inline or hoisted to a `const`. An interpolation
// `${…}` is consumed whole, so its brace does not end the value.
const INLINE_DECLARATION =
  /(?:^|[{,\s])(fontSize|lineHeight|letterSpacing|textTransform|fontWeight|font)\s*:\s*((?:\$\{[^}]*\}|[^,}\n])*)/g;

/** Comments blanked, keeping every line, so prose about a declaration is not one. */
const withoutComments = (source: string) =>
  source.replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    comment.replace(/[^\n]/g, " "),
  );

const lineOf = (text: string, index: number) =>
  text.slice(0, index).split("\n").length;

/**
 * The selector list of the rule a declaration stands in. A nested rule reads
 * its own selector, not the `@media` around it.
 */
function selectorOf(text: string, index: number): string {
  const open = text.lastIndexOf("{", index);
  if (open < 0) return "";
  const before = text.slice(0, open);
  const ended = Math.max(
    before.lastIndexOf("}"),
    before.lastIndexOf("{"),
    before.lastIndexOf(";"),
  );
  return before
    .slice(ended + 1)
    .replace(/\s+/g, " ")
    .trim();
}

/** A style object's value with its quotes removed. */
const unquoted = (value: string) =>
  value
    .trim()
    .replace(/^["'`]/, "")
    .replace(/["'`]$/, "");

/**
 * Whether a CSS declaration may say what it says. A size, leading or tracking
 * has no token of its own: it exists only inside a `font` shorthand.
 */
function cssDeclarationHolds(
  file: string,
  selector: string,
  property: string,
  value: string,
): boolean {
  if (value === INHERIT) return true;
  if (property === "font") return TYPE_TOKEN.test(value);
  if (property === "font-weight") return WEIGHT_TOKEN.test(value);
  // These three carry figure shapes and ligatures too; only capitals count.
  if (property === "font-variant-caps") return value === "normal";
  if (property === "font-variant") return !CAPS_KEYWORD.test(value);
  if (property === "font-feature-settings") return !CAPS_FEATURE.test(value);
  if (!RESETS.includes(file)) return false;
  if (property === "text-transform") return value === "none";
  if (property === "line-height") {
    return value === "0" && NEUTRALISED.test(selector);
  }
  return false;
}

// A `var()` whose name is interpolated resolves to a custom property, which is
// a token. The type specimen page renders its own list of tokens this way.
const isComputedToken = (value: string) =>
  /^var\(\s*\$\{[^}]+\}\s*\)$/.test(value);

function inlineDeclarationHolds(property: string, value: string): boolean {
  if (value === INHERIT) return true;
  if (property === "font")
    return TYPE_TOKEN.test(value) || isComputedToken(value);
  if (property === "fontWeight") return WEIGHT_TOKEN.test(value);
  return false;
}

/** Every declaration in one stylesheet that spells type by value. */
export function cssFindings(file: string, source: string): string[] {
  const text = withoutComments(source);
  return [...text.matchAll(CSS_DECLARATION)].flatMap((match) => {
    const [property, value] = [match[1], match[2].trim()];
    const selector = selectorOf(text, match.index);
    if (cssDeclarationHolds(file, selector, property, value)) return [];
    return [`${file}:${lineOf(text, match.index)}: ${property}: ${value}`];
  });
}

/** Every style-object declaration in one module that spells type by value. */
export function inlineFindings(file: string, source: string): string[] {
  return [...source.matchAll(INLINE_DECLARATION)].flatMap((match) => {
    const [property, raw] = [match[1], match[2]];
    if (inlineDeclarationHolds(property, unquoted(raw))) return [];
    return [
      `${file}:${lineOf(source, match.index)}: ${property}: ${raw.trim()}`,
    ];
  });
}

const UPPERCASING = new Set(["toUpperCase", "toLocaleUpperCase"]);

/**
 * Whether an expression stands where JSX renders it: a child, an attribute
 * value, or a template piece in either place. A comparison or a map key is
 * data handling and renders nothing.
 */
function isRendered(call: ts.Node): boolean {
  let here: ts.Node = call;
  let parent: ts.Node | undefined = here.parent;
  while (
    parent &&
    (ts.isParenthesizedExpression(parent) ||
      ts.isTemplateSpan(parent) ||
      ts.isTemplateExpression(parent))
  ) {
    here = parent;
    parent = here.parent;
  }
  if (!parent || !ts.isJsxExpression(parent)) return false;
  const holder = parent.parent;
  return (
    holder !== undefined &&
    (ts.isJsxAttribute(holder) ||
      ts.isJsxElement(holder) ||
      ts.isJsxFragment(holder) ||
      ts.isJsxSelfClosingElement(holder))
  );
}

/** Every uppercased string one module puts on the screen. */
export function uppercasingFindings(
  file: string,
  source: ts.SourceFile,
): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      UPPERCASING.has(node.expression.name.text) &&
      isRendered(node)
    ) {
      const { line } = source.getLineAndCharacterOfPosition(
        node.getStart(source),
      );
      found.push(
        `${file}:${line + 1}: ${node.getText(source).replace(/\s+/g, " ")}`,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const isSheet = (file: CensusFile) => file.rel.endsWith(".css");
const isModule = (file: CensusFile) =>
  /\.tsx?$/.test(file.rel) && !/\.test\.tsx?$/.test(file.rel);

export const rule: CensusRule = {
  name: "type-source",
  corpus: (tree) =>
    tree.files.filter(
      (file) => file.area === "src" && (isSheet(file) || isModule(file)),
    ),
  floors: (corpus) => {
    const sheets = corpus.filter(isSheet).map((file) => file.rel);
    const modules = corpus.filter(isModule).map((file) => file.rel);
    const components = modules.filter((rel) => rel.endsWith(".tsx"));
    return [
      ...[RESETS[0], SOURCE]
        .filter((rel) => !sheets.includes(rel))
        .map((rel) => `${rel} was not read`),
      ...(sheets.length >= 100 ? [] : ["fewer than 100 stylesheets were read"]),
      ...(modules.includes("src/design-system/provider-mark.tsx")
        ? []
        : ["src/design-system/provider-mark.tsx was not read"]),
      ...(modules.length >= 100 ? [] : ["fewer than 100 modules were read"]),
      ...(components.length >= 100
        ? []
        : ["fewer than 100 .tsx modules were read"]),
    ];
  },
  visit: (file) => {
    if (isSheet(file)) {
      return file.rel === SOURCE ? [] : cssFindings(file.rel, file.text);
    }
    return [
      ...inlineFindings(file.rel, file.text),
      ...(file.rel.endsWith(".tsx") && file.source
        ? uppercasingFindings(file.rel, file.source)
        : []),
    ];
  },
  advice:
    "Type comes from tokens.css and from nowhere else: wear a `--font*` token " +
    "(a `font` shorthand, or a `--fontWeight*` for a weight) or inherit. Draw " +
    "no capitals by casing a string or by small caps.",
};
