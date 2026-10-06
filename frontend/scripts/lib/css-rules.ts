// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every CSS rule under a tree, and what its selector says about the element it
// paints. Read as text rather than through a parser: the sheets here are flat,
// which `rules()` holds, and a selector is read by one depth-aware tokenizer.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { extensionLayers, filesMatching } from "./source-tree";

// Every sheet the bundle ships: the core's and each extension's frontend layer,
// each directory read once however many links reach it.
export function appStylesheets(frontendRoot: string): string[] {
  const seen = new Set<string>();
  return [
    join(frontendRoot, "src"),
    ...extensionLayers(join(frontendRoot, "..", "extensions")),
  ].flatMap((dir) => filesMatching(dir, /\.css$/, seen));
}

export type Rule = { file: string; selector: string; body: string };

export function rules(root: string): Rule[] {
  const sheets = filesMatching(root, /\.css$/);
  if (sheets.length === 0) {
    throw new Error(
      `no stylesheet under ${root} — a rule walk over none reports a clean tree`,
    );
  }
  return rulesOf(sheets);
}

export function rulesOf(sheets: readonly string[]): Rule[] {
  const all: Rule[] = [];
  for (const file of sheets) {
    const sheet = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    assertFlat(file, sheet);
    // Innermost brace pairs, so a rule nested in an @media is found as
    // itself and the query around it never matches as a selector.
    for (const [, selector, body] of sheet.matchAll(/([^{}]*)\{([^{}]*)\}/g)) {
      for (const one of selectorList(selector)) {
        all.push({ file, selector: one, body });
      }
    }
  }
  return all;
}

// A nested style rule would read as its parent's declarations turned selector.
function assertFlat(file: string, sheet: string): void {
  const styleBlocks: boolean[] = [];
  let from = 0;
  for (let at = 0; at < sheet.length; at++) {
    if (sheet[at] === "{") {
      if (styleBlocks.at(-1) === true) {
        throw new Error(
          `${file} nests a rule inside a rule; rules() reads flat CSS only`,
        );
      }
      styleBlocks.push(!sheet.slice(from, at).trim().startsWith("@"));
    } else if (sheet[at] === "}") {
      styleBlocks.pop();
    }
    if ("{};".includes(sheet[at])) from = at + 1;
  }
}

const NESTING = new Map([
  ["(", 1],
  ["[", 1],
  [")", -1],
  ["]", -1],
]);

type Scan = { topLevel: number[]; balanced: boolean };

function scan(text: string): Scan {
  const topLevel: number[] = [];
  let depth = 0;
  let quote = "";
  for (let at = 0; at < text.length && depth >= 0; at++) {
    const char = text[at];
    if (char === "\\") {
      at++;
    } else if (quote !== "" || "\"'".includes(char)) {
      quote = quoteAfter(quote, char);
    } else {
      depth += NESTING.get(char) ?? 0;
      if (depth === 0 && !NESTING.has(char)) topLevel.push(at);
    }
  }
  return { topLevel, balanced: depth === 0 && quote === "" };
}

export function isBalanced(text: string): boolean {
  return scan(text).balanced;
}

function quoteAfter(quote: string, char: string): string {
  if (quote === "") return char;
  return char === quote ? "" : quote;
}

type Piece = { text: string; separator: string };

function pieces(text: string, separators: string): Piece[] {
  const { topLevel, balanced } = scan(text);
  if (!balanced) {
    throw new Error(`unbalanced (), [] or quote in CSS text: ${text}`);
  }
  const out: Piece[] = [];
  let from = 0;
  for (const at of topLevel) {
    if (separators.includes(text[at])) {
      out.push({ text: text.slice(from, at), separator: text[at] });
      from = at + 1;
    }
  }
  out.push({ text: text.slice(from), separator: "" });
  return out;
}

// A separator inside (), [] or quotes does not split: `:is(.a, .b)` is one.
export function splitTopLevel(text: string, separators: string): string[] {
  return pieces(text, separators)
    .map((piece) => piece.text.trim())
    .filter(Boolean);
}

export function selectorList(text: string): string[] {
  return splitTopLevel(text, ",");
}

export function resolveNesting(
  outer: readonly string[],
  list: string,
): string[] {
  return selectorList(list).flatMap((inner) =>
    outer.length === 0
      ? [inner]
      : outer.map((parent) =>
          inner.includes("&")
            ? inner.replaceAll("&", parent)
            : `${parent} ${inner}`,
        ),
  );
}

const ALTERNATION = /^(?:is|where|matches|-webkit-any)\(/;
const ATTRIBUTE = /\[(?:[^\]"']|"[^"]*"|'[^']*')*\]/g;
const CLASS = /\.((?:[\w-]|\\.)+)/g;

// A functional pseudo-class names something OTHER than the element it is
// written on, so its argument is not part of that element's classes:
// `:not(.btn)` would otherwise make `btn` a class the chip carries, and the
// subtree search would then find nothing at all. An `:is()` adds the classes
// every alternative shares, which is what the element MUST carry.
export function classesOf(compound: string): Set<string> {
  const names = new Set<string>();
  for (const part of splitTopLevel(compound, ":")) {
    const functional = /^[\w-]+\(/.test(part);
    const open = functional ? part.indexOf("(") : part.length;
    const close = functional ? part.lastIndexOf(")") : part.length;
    const written = (part.slice(0, open) + part.slice(close + 1)).replace(
      ATTRIBUTE,
      "",
    );
    for (const [, name] of written.matchAll(CLASS)) {
      names.add(name.replaceAll(/\\(.)/g, "$1"));
    }
    if (ALTERNATION.test(part)) {
      for (const name of everyAlternative(part.slice(open + 1, close))) {
        names.add(name);
      }
    }
  }
  return names;
}

function everyAlternative(list: string): string[] {
  const [first, ...rest] = selectorList(list).map(subjectClasses);
  return [...(first ?? [])].filter((name) =>
    rest.every((other) => other.has(name)),
  );
}

const COMBINATORS = " \t\n\r\f>+~";

type Chain = { compounds: string[]; combinators: string[] };

function chain(selector: string): Chain {
  const links: Chain = { compounds: [], combinators: [] };
  let combinator = " ";
  for (const piece of pieces(selector, COMBINATORS)) {
    const compound = piece.text.trim();
    if (compound !== "") {
      if (links.compounds.length > 0) links.combinators.push(combinator);
      links.compounds.push(compound);
      combinator = " ";
    }
    if (piece.separator.trim() !== "") combinator = piece.separator;
  }
  return links;
}

function joined({ compounds, combinators }: Chain): string {
  return compounds.reduce((out, compound, index) => {
    const combinator = combinators[index - 1] ?? " ";
    return `${out}${combinator === " " ? " " : ` ${combinator} `}${compound}`;
  });
}

export function compounds(selector: string): string[] {
  return chain(selector).compounds;
}

const MAX_ALTERNATIVES = 256;

// A census of what a rule MIGHT paint reads each alternative, not their overlap.
export function alternativesOf(selector: string): string[] {
  const links = chain(selector);
  const at = links.compounds.findIndex((one) => alternation(one) !== undefined);
  const split = alternation(links.compounds[at] ?? "");
  if (split === undefined) return [selector];
  const out = selectorList(split.list).flatMap((alternative) => {
    const inner = chain(alternative);
    const subject = mergedCompound(
      split.before,
      inner.compounds.at(-1) ?? "",
      split.after,
    );
    if (subject === undefined) return [];
    return alternativesOf(
      joined({
        compounds: [
          ...links.compounds.slice(0, at),
          ...inner.compounds.slice(0, -1),
          subject,
          ...links.compounds.slice(at + 1),
        ],
        combinators: [
          ...links.combinators.slice(0, at),
          ...inner.combinators,
          ...links.combinators.slice(at),
        ],
      }),
    );
  });
  if (out.length > MAX_ALTERNATIVES) {
    throw new Error(
      `${selector} distributes past ${MAX_ALTERNATIVES} selectors`,
    );
  }
  return out;
}

type Alternation = { before: string; list: string; after: string };

function alternation(compound: string): Alternation | undefined {
  const [base, ...pseudos] = pieces(compound, ":").map((piece) => piece.text);
  let before = base ?? "";
  for (const [index, pseudo] of pseudos.entries()) {
    if (ALTERNATION.test(pseudo)) {
      const close = pseudo.lastIndexOf(")");
      const rest = pseudos.slice(index + 1).map((one) => `:${one}`);
      return {
        before,
        list: pseudo.slice(pseudo.indexOf("(") + 1, close),
        after: pseudo.slice(close + 1) + rest.join(""),
      };
    }
    before += `:${pseudo}`;
  }
  return undefined;
}

const TYPE = /^(?:[a-zA-Z][\w-]*|\*)/;

// A type selector has to lead its compound, whichever side wrote it; two
// different ones match no element at all.
function mergedCompound(
  before: string,
  subject: string,
  after: string,
): string | undefined {
  const types = [TYPE.exec(before)?.[0], TYPE.exec(subject)?.[0]];
  const named = types.filter(
    (one): one is string => one !== undefined && one !== "*",
  );
  if (new Set(named.map((one) => one.toLowerCase())).size > 1) return undefined;
  const type = named[0] ?? types.find((one) => one !== undefined) ?? "";
  return type + before.replace(TYPE, "") + subject.replace(TYPE, "") + after;
}

// The SUBJECT of a selector: the compound the rule actually paints, which is
// the last one. `.palette-row .type` styles the chip, not the row — reading
// its first compound instead put every `.palette-row` descendant inside a
// chip it is only a sibling of.
export function subjectOf(selector: string): string {
  return compounds(selector).at(-1) ?? "";
}

export function subjectsOf(selector: string): string[] {
  return alternativesOf(selector).map(subjectOf);
}

export function subjectClasses(selector: string): Set<string> {
  return classesOf(subjectOf(selector));
}

// The custom properties a rule's body sets as its INK.
//
// `(?:^|[;{\s])` is what keeps this off --*-color: the character before a
// longhand's `color:` is always a hyphen.
export function inks(body: string): string[] {
  return colorValues(body).flatMap(
    (value) => /^var\((--[\w-]+)\)/.exec(value)?.[1] ?? [],
  );
}

// Every value a rule's body gives `property`, whatever it is spelled as: a
// fallback, a `color-mix()` or a second declaration all read here.
export function declaredValues(body: string, property: string): string[] {
  const name = property.replaceAll("-", "\\-");
  return [
    ...body.matchAll(new RegExp(`(?:^|[;{\\s])${name}\\s*:\\s*([^;]+)`, "g")),
  ].map(([, value]) => value.trim());
}

export function colorValues(body: string): string[] {
  return declaredValues(body, "color");
}
