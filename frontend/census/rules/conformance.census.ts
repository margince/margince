// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The source-wide design gates: three type families (Outfit, Geist, Geist
// Mono), and every colour from a token. Copy comes from the i18n catalogs, no
// emoji, and one spelling each for the button and the card.

import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

const allowedFamilies = new Set([
  "Outfit",
  "Geist",
  "Geist Mono",
  // Stack fallbacks named in the token definitions.
  "system-ui",
  "sans-serif",
  "ui-monospace",
  "monospace",
]);

// A colour written out rather than read from a token: any hex form CSS accepts
// or a raw colour function.
export const LITERAL_COLOUR = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch)\(/;

const TEST = /\.test\.tsx?$/;

// The value of a JSX attribute when the source states it as a literal string.
// A computed value has no value this rule can read, and returns undefined.
function literalAttributeValue(initializer: ts.Node): string | undefined {
  if (
    ts.isStringLiteral(initializer) ||
    ts.isNoSubstitutionTemplateLiteral(initializer)
  ) {
    return initializer.text;
  }
  if (ts.isJsxExpression(initializer) && initializer.expression !== undefined) {
    return literalAttributeValue(initializer.expression);
  }
  return undefined;
}

// Blanks every span a pattern matches, keeping the newlines for line numbers.
function blankSpans(text: string, comment: RegExp): string {
  return text.replace(comment, (span) => span.replace(/[^\n]/g, " "));
}

/**
 * The source with its comments blanked, newlines kept. An issue reference in
 * prose is four hex digits to the pattern, so prose must not be read.
 *
 * TypeScript comments come from the parser, because `//` sits inside every URL.
 * The walk reaches every token, which is what finds a `{/* … *\/}` in JSX.
 */
export function scannableSource(file: string, text: string): string {
  if (!/\.tsx?$/.test(file)) {
    return blankSpans(
      text,
      file.endsWith(".css") ? /\/\*[\s\S]*?\*\//g : /<!--[\s\S]*?-->/g,
    );
  }
  const parsed = parseSource(file, text);
  const chars = text.split("");
  const blank = ({ pos, end }: ts.CommentRange): void => {
    for (let index = pos; index < end; index++) {
      if (chars[index] !== "\n") chars[index] = " ";
    }
  };
  // The parser reports a comment that shares a line with code before it as
  // trailing only, so both kinds are read.
  const strip = (node: ts.Node): void => {
    ts.getLeadingCommentRanges(text, node.pos)?.forEach(blank);
    ts.getTrailingCommentRanges(text, node.end)?.forEach(blank);
    for (const child of node.getChildren(parsed)) strip(child);
  };
  strip(parsed);
  return chars.join("");
}

/**
 * Every type family named in one source text, as `font-family:` or as an
 * inline `fontFamily:` in any quote. A token reference, a family built at run
 * time and `inherit` name nothing this rule can judge.
 */
export function familiesIn(text: string): string[] {
  const found: string[] = [];
  for (const [, declared, quoted, templated] of text.matchAll(
    /font-family\s*:\s*([^;}]+)|fontFamily\s*:\s*(?:["']([^"']+)["']|`([^`]+)`)/g,
  )) {
    for (const family of (declared ?? quoted ?? templated ?? "").split(",")) {
      const name = family.trim().replace(/^["'`]|["'`]$/g, "");
      if (name === "" || name === "inherit") continue;
      if (name.startsWith("var(") || name.includes("${")) continue;
      found.push(name);
    }
  }
  return found;
}

/** The colour lines of one file, as 1-based line numbers. */
// Blanking comments only removes matches, so a text with none skips the parse.
export const colourLines = (file: string, text: string): number[] =>
  LITERAL_COLOUR.test(text)
    ? scannableSource(file, text)
        .split("\n")
        .flatMap((line, index) =>
          LITERAL_COLOUR.test(line) ? [index + 1] : [],
        )
    : [];

const lineOf = (source: ts.SourceFile, node: ts.Node) =>
  source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

// Every literal fragment a className can evaluate to, so a conditional or an
// interpolated class list is read too.
function classFragments(attribute: ts.JsxAttribute): string[] {
  const fragments: string[] = [];
  const collect = (child: ts.Node) => {
    if (
      ts.isStringLiteral(child) ||
      ts.isTemplateLiteralToken(child) ||
      ts.isJsxText(child)
    ) {
      fragments.push(child.text);
    }
    ts.forEachChild(child, collect);
  };
  if (attribute.initializer) collect(attribute.initializer);
  return fragments;
}

const hasClass = (attribute: ts.JsxAttribute, names: readonly string[]) =>
  classFragments(attribute).some((fragment) =>
    fragment.split(/\s+/).some((token) => names.includes(token)),
  );

// A role other than `status` is one Card cannot express, so such a surface has
// no component to reach for. A computed role is not an exemption.
const declaresOtherRole = (attribute: ts.JsxAttribute) =>
  attribute.parent.properties.some((property) => {
    if (
      !ts.isJsxAttribute(property) ||
      property.name.getText() !== "role" ||
      property.initializer === undefined
    ) {
      return false;
    }
    const role = literalAttributeValue(property.initializer);
    return role !== undefined && role !== "status";
  });

// An anchor styled as a button has no component: Button renders a `<button>`.
const ANCHORS = new Set(["a", "OffsiteLink", "AppLink"]);

/** Hand-rolled `btn` and `card` classes in one .tsx file. */
export function handRolledClasses(
  rel: string,
  source: ts.SourceFile,
): string[] {
  const ownsButton = rel.endsWith("design-system/button.tsx");
  const ownsCard = rel.endsWith("design-system/atoms.tsx");
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isJsxAttribute(node) && node.name.getText() === "className") {
      const tag = node.parent.parent.tagName.getText(source);
      const at = `${rel}:${lineOf(source, node)} <${tag}>`;
      if (!ownsButton && !ANCHORS.has(tag) && hasClass(node, ["btn"])) {
        found.push(
          `${at} spells the btn class by hand — import Button from design-system/atoms`,
        );
      }
      if (
        !ownsCard &&
        hasClass(node, ["card", "card-inset"]) &&
        !declaresOtherRole(node)
      ) {
        found.push(
          `${at} spells the card class by hand — import Card from design-system/atoms`,
        );
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const USER_FACING_ATTRS = new Set([
  "aria-label",
  "title",
  "placeholder",
  "alt",
]);
const hasWords = (text: string) => /[A-Za-z]{2,}/.test(text);

/** Copy written inline in JSX rather than read from the i18n catalogs. */
export function inlineCopy(rel: string, source: ts.SourceFile): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isJsxText(node) && hasWords(node.text)) {
      found.push(
        `${rel}:${lineOf(source, node)} JSX text "${node.text.trim()}"`,
      );
    }
    if (
      ts.isJsxAttribute(node) &&
      USER_FACING_ATTRS.has(node.name.getText(source)) &&
      node.initializer &&
      ts.isStringLiteral(node.initializer) &&
      hasWords(node.initializer.text)
    ) {
      found.push(
        `${rel}:${lineOf(source, node)} ${node.name.getText(source)}="${node.initializer.text}"`,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const EMOJI = /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]/u;

/** Emoji glyphs in a string or JSX text: an icon is a lucide or Material glyph. */
export function emojiIn(rel: string, source: ts.SourceFile): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      (ts.isStringLiteral(node) ||
        ts.isNoSubstitutionTemplateLiteral(node) ||
        ts.isJsxText(node)) &&
      EMOJI.test(node.text)
    ) {
      found.push(`${rel}: emoji in "${node.text.trim()}"`);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const familyFindings = (rel: string, path: string, text: string) =>
  familiesIn(familiesIn(text).length > 0 ? scannableSource(path, text) : "")
    .filter((name) => !allowedFamilies.has(name))
    .map(
      (name) =>
        `${rel}: font-family "${name}" is outside the three-family rule`,
    );

// Stories are catalog fixtures and keep literal demo copy. story-hosts.tsx is
// Storybook-only scaffolding that cannot carry a .stories.tsx name; keep it a
// named file rather than a pattern.
const copyExempt = (rel: string) =>
  /\.(test|stories)\.tsx$/.test(rel) ||
  rel.endsWith("mcp-apps/story-hosts.tsx");

// tokens.css is where literal colours live, and an HTML page's theme-color
// meta cannot read a custom property. provider-mark.tsx draws another
// company's sign-in marks. tokens-testing.ts parses the `rgba(` grammar.
// scripts/check-ds-purity.sh repeats both named files.
const colourExempt = (rel: string) =>
  rel.endsWith("tokens.css") ||
  rel.endsWith("index.html") ||
  rel.endsWith("provider-mark.tsx") ||
  TEST.test(rel) ||
  rel.endsWith("tokens-testing.ts");

/** Every finding the gates make in one file of the corpus. */
export function conformanceFindings(file: CensusFile): string[] {
  const { rel, path, text, source } = file;
  const found: string[] = [];
  if (!TEST.test(rel)) found.push(...familyFindings(rel, path, text));
  if (!colourExempt(rel)) {
    found.push(
      ...colourLines(path, text).map(
        (line) => `${rel}:${line} hard-codes a colour — read it from a token`,
      ),
    );
  }
  if (!source) return found;
  if (rel.endsWith(".tsx")) {
    if (!copyExempt(rel)) found.push(...inlineCopy(rel, source));
    found.push(...handRolledClasses(rel, source));
  }
  if (!TEST.test(rel) && !rel.endsWith(".d.ts")) {
    found.push(...emojiIn(rel, source));
  }
  return found;
}

// The app's own index.html sits at the top of frontend/.
const isSource = (file: CensusFile) =>
  ((file.area === "src" || file.area === "extensions") &&
    /\.(css|tsx?|html)$/.test(file.rel)) ||
  file.rel === "index.html";

export const rule: CensusRule = {
  name: "conformance",
  corpus: (tree) => tree.files.filter(isSource),
  floors: (corpus) => [
    ...(corpus.length > 500 ? [] : ["fewer than 500 source files were read"]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the census reached no extension frontend layer"]),
    ...(corpus.filter((file) => file.rel.endsWith(".html")).length > 1
      ? []
      : ["fewer than two HTML pages were read"]),
  ],
  visit: conformanceFindings,
  advice:
    "Type families are Outfit, Geist and Geist Mono. Colours read a token; " +
    "literal colours live only in tokens.css. Copy comes from the i18n " +
    "catalogs. No emoji: use a lucide or Material glyph. Buttons render " +
    "through Button and cards through Card.",
};
