// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionLayers,
  filesMatching,
  parseSource,
} from "../../scripts/lib/source-tree";

// Mono is for code, and only for code.
//
// The code face is Geist Mono, and the one place an element wears it is an
// element that IS code: `code`, `pre`, `samp`, or `.code-block`. An id, a key, a
// model name, a timestamp and an amount are all read in the body face; a figure
// that has to line up in a column gets `font-variant-numeric: tabular-nums`
// (`.t-num`), which aligns the digits without changing the family. The mono
// face had been doing two jobs — aligning columns and marking "machine text" —
// and the second made every screen that carried an identifier look like a
// terminal beside the screen that did not.
//
// Three spellings reintroduce it, and each has an arm:
//  (a) the `t-mono` class, in markup, a stylesheet or a selector string;
//  (b) a `font-family` or `font` declaration naming a mono family on a rule
//      whose selectors do not ALL target code — plus any custom property
//      carrying one, except the `--fontFamilyMono` token itself in tokens.css;
//  (c) the same family inside a TypeScript string: an inline `fontFamily`, a
//      style string, or a constant handed to either.
//
// scripts/check-font-lock.sh carries the same three arms as a grep, so the rule
// holds even if this file regresses.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

// Every source a bundler, Storybook or Playwright reads. Tests are left out
// because a test naming the class or the family is the assertion, not the
// defect — this file's own fixtures are exactly that. Declaration files are left
// out because a `.d.ts` holds types and nothing in one can render a class or set
// a face; the generated contract types are three megabytes of that.
const SOURCE = /\.(css|html|[cm]?[jt]sx?)$/;

function corpus(): string[] {
  return [
    ...filesMatching(join(frontendRoot, "src"), SOURCE),
    ...filesMatching(join(frontendRoot, "e2e"), SOURCE),
    ...filesMatching(join(frontendRoot, ".storybook"), SOURCE),
    join(frontendRoot, "index.html"),
    ...extensionLayers(join(frontendRoot, "..", "extensions")).flatMap(
      (layer) => filesMatching(layer, SOURCE),
    ),
  ].filter((file) => !/\.test\.[cm]?[jt]sx?$|\.d\.ts$/.test(file));
}

// `monospace` also matches inside `ui-monospace`, which is intended: a hyphen is
// a word boundary.
const MONO_FAMILY = /var\(\s*--fontFamilyMono\s*\)|Geist Mono|\bmonospace\b/i;
const T_MONO = /(?<![\w-])t-mono(?![\w-])/;

// Splits on a character at the top level only, so a comma inside `:is(a, b)` or
// a space inside `[data-x="a b"]` does not cut a selector in two.
function splitTopLevel(text: string, at: RegExp): string[] {
  const parts: string[] = [];
  let depth = 0;
  let current = "";
  for (const char of text) {
    if (char === "(" || char === "[") depth++;
    if (char === ")" || char === "]") depth--;
    if (depth === 0 && at.test(char)) {
      parts.push(current);
      current = "";
    } else {
      current += char;
    }
  }
  parts.push(current);
  return parts.map((part) => part.trim()).filter((part) => part !== "");
}

// A nested selector is resolved against its parent the way CSS nesting does:
// `&` stands for the parent, and a selector without one is its descendant.
function resolveSelectors(prelude: string, parents: string[]): string[] {
  const own = splitTopLevel(prelude, /,/);
  if (parents.length === 0) return own;
  return parents.flatMap((parent) =>
    own.map((selector) =>
      selector.includes("&")
        ? selector.replaceAll("&", parent)
        : `${parent} ${selector}`,
    ),
  );
}

/**
 * Whether a selector's SUBJECT — the element the declarations land on — is
 * code. `.foo code` is; `code .foo` is not, because the rule dresses `.foo`.
 */
function targetsCode(selector: string): boolean {
  const subject = splitTopLevel(selector, /[\s>+~]/).at(-1) ?? "";
  const compound = splitTopLevel(subject, /:/)[0] ?? "";
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
 * selectors it applies to. A declaration outside any rule — a `style`
 * attribute's contents — applies to no selector, so it can never target code.
 */
function monoDeclarations(css: string): MonoDeclaration[] {
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
      open.push(
        prelude.startsWith("@") ? parents : resolveSelectors(prelude, parents),
      );
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

function isAllowed(file: string, declaration: MonoDeclaration): boolean {
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

// Strings only: a comment discussing the rule is not breaking it, and JSX text
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
      // A string naming the family is either a stylesheet in a string, judged
      // rule by rule, or a bare value — an inline `fontFamily`, a constant —
      // that no rule scopes to code at all.
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
function monoViolations(file: string, text: string): string[] {
  if (file.endsWith(".css")) return stylesheetViolations(file, text, 0);
  if (file.endsWith(".html")) return htmlViolations(file, text);
  return scriptViolations(file, text);
}

describe("mono is for code", { timeout: 60_000 }, () => {
  const files = corpus();

  it("reads the tree it claims, down to the one rule that sets the code face", () => {
    const names = files.map((file) => relative(frontendRoot, file));
    expect(names).toContain("src/design-system/base.css");
    expect(names).toContain("src/design-system/tokens.css");
    expect(names).toContain("src/mcp-apps/view.css");
    expect(names).toContain("index.html");
    // The base rule is the proof the parser reaches a real declaration: a
    // parser that read nothing would report this tree clean for free.
    const base = readFileSync(
      join(frontendRoot, "src/design-system/base.css"),
      "utf8",
    );
    const codeRule = monoDeclarations(base);
    expect(codeRule.length).toBeGreaterThan(0);
    expect(codeRule.every((entry) => isAllowed("base.css", entry))).toBe(true);
  });

  it("sets the code face on code alone, and nowhere carries t-mono", () => {
    const violations = files.flatMap((file) =>
      monoViolations(relative(frontendRoot, file), readFileSync(file, "utf8")),
    );
    expect(violations, violations.join("\n")).toEqual([]);
  });

  it("refuses mono on a rule that does not dress code", () => {
    const refused = (css: string, file = "src/screens/x.css") =>
      monoViolations(file, css).length > 0;
    expect(refused(".foo { font-family: var(--fontFamilyMono); }")).toBe(true);
    expect(refused('.foo { font: 12px/1 "Geist Mono", monospace; }')).toBe(
      true,
    );
    expect(refused(".foo { font-family: ui-monospace; }")).toBe(true);
    // One selector in the list that is not code puts mono on that element.
    expect(refused(".foo, code { font-family: var(--fontFamilyMono); }")).toBe(
      true,
    );
    // The subject is what is dressed: here it is the label, not the code.
    expect(refused("code .label { font-family: var(--fontFamilyMono); }")).toBe(
      true,
    );
    expect(
      refused(".code-block .label { font-family: var(--fontFamilyMono); }"),
    ).toBe(true);
    expect(
      refused(".a { code & { font-family: var(--fontFamilyMono); } }"),
    ).toBe(true);
    expect(
      refused("@media (width > 1px) { .a { font-family: monospace; } }"),
    ).toBe(true);
    // A token of its own is a second way to spell the face.
    expect(refused(":root { --code-face: var(--fontFamilyMono); }")).toBe(true);
    expect(refused(':root { --fontFamilyMono: "Geist Mono"; }')).toBe(true);
    expect(refused(".t-mono { color: red; }")).toBe(true);
  });

  it("allows mono on code, pre, samp and .code-block, and on the token", () => {
    const allowed = (css: string, file = "src/screens/x.css") =>
      monoViolations(file, css);
    expect(
      allowed(".foo code { font-family: var(--fontFamilyMono); }"),
    ).toEqual([]);
    expect(
      allowed("code, pre, samp { font-family: var(--fontFamilyMono); }"),
    ).toEqual([]);
    expect(allowed("pre.code-block:hover { font-family: monospace; }")).toEqual(
      [],
    );
    expect(
      allowed(".code-block { font-family: var(--fontFamilyMono); }"),
    ).toEqual([]);
    expect(
      allowed(".a { & code { font-family: var(--fontFamilyMono); } }"),
    ).toEqual([]);
    expect(
      allowed("@media (width > 1px) { pre { font-family: monospace; } }"),
    ).toEqual([]);
    expect(
      allowed(
        ':root { --fontFamilyMono: "Geist Mono", ui-monospace, monospace; }',
        "src/design-system/tokens.css",
      ),
    ).toEqual([]);
    // A comment discussing the rule is not breaking it.
    expect(
      allowed("/* .t-mono and font-family: monospace were */ .a { }"),
    ).toEqual([]);
  });

  it("reads the class and the family in markup and in strings", () => {
    const refused = (source: string, file = "src/screens/x.tsx") =>
      monoViolations(file, source).length > 0;
    expect(refused('export const A = () => <span className="t-mono" />;')).toBe(
      true,
    );
    expect(
      refused(
        // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code — the `${x}` is the template literal the scan has to read
        "export const A = (x: string) => <b className={`${x} t-mono`} />;",
      ),
    ).toBe(true);
    expect(
      refused(
        'export const A = () => <b style={{ fontFamily: "var(--fontFamilyMono)" }} />;',
      ),
    ).toBe(true);
    expect(refused("const face = { fontFamily: 'monospace' };")).toBe(true);
    expect(refused('const sheet = ".a { font-family: Geist Mono; }";')).toBe(
      true,
    );
    expect(
      refused('<div style="font-family: monospace">x</div>', "index.html"),
    ).toBe(true);
    expect(refused('<b class="t-mono">1</b>', "index.html")).toBe(true);

    const allowed = (source: string, file = "src/screens/x.tsx") =>
      monoViolations(file, source);
    expect(allowed('const a = "t-monochrome t-num";')).toEqual([]);
    expect(
      allowed("// the t-mono class and var(--fontFamilyMono) are gone"),
    ).toEqual([]);
    expect(
      allowed(
        'const sheet = "pre code { font-family: var(--fontFamilyMono); }";',
      ),
    ).toEqual([]);
    expect(
      allowed("<style>pre { font-family: monospace; }</style>", "index.html"),
    ).toEqual([]);
  });
});
