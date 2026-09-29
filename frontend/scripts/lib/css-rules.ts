// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every CSS rule under a tree, and the three questions a selector is asked
// about it. Read as text rather than through a parser: the sheets here are
// hand-written and flat, and the whole corpus is one regex pass.

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

export function stylesheets(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : stylesheets(path);
    }
    return path.endsWith(".css") ? [path] : [];
  });
}

export type Rule = { file: string; selector: string; body: string };

export function rules(root: string): Rule[] {
  const sheets = stylesheets(root);
  if (sheets.length === 0) {
    throw new Error(
      `no stylesheet under ${root} — a rule walk over none reports a clean tree`,
    );
  }
  const all: Rule[] = [];
  for (const file of sheets) {
    const sheet = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
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

const NESTING = new Map([
  ["(", 1],
  ["[", 1],
  [")", -1],
  ["]", -1],
]);

// A separator inside (), [] or quotes does not split: `:is(.a, .b)` is one.
export function splitTopLevel(text: string, separators: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let quote = "";
  let from = 0;
  for (let at = 0; at < text.length; at++) {
    const char = text[at];
    if (char === "\\") {
      at++;
    } else if (quote !== "") {
      quote = char === quote ? "" : quote;
    } else if ("\"'".includes(char)) {
      quote = char;
    } else if (depth === 0 && separators.includes(char)) {
      parts.push(text.slice(from, at));
      from = at + 1;
    } else {
      depth += NESTING.get(char) ?? 0;
    }
  }
  parts.push(text.slice(from));
  return parts.map((part) => part.trim()).filter(Boolean);
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

// A functional pseudo-class names something OTHER than the element it is
// written on, so its argument is not part of that element's classes:
// `:not(.btn)` would otherwise make `btn` a class the chip carries, and the
// subtree search would then find nothing at all. `:is()` and `:where()` name
// the element itself, so it carries whatever every alternative carries.
export function classesOf(compound: string): Set<string> {
  const names = new Set<string>();
  for (const part of splitTopLevel(compound, ":")) {
    const functional = /^[\w-]+\(/.test(part);
    const open = functional ? part.indexOf("(") : part.length;
    const close = functional ? part.lastIndexOf(")") : part.length;
    const written = part.slice(0, open) + part.slice(close + 1);
    for (const [, name] of written.matchAll(/\.([\w-]+)/g)) names.add(name);
    if (/^(?:is|where)\(/.test(part)) {
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

export function compounds(selector: string): string[] {
  return splitTopLevel(selector, " \t\n\r\f>+~");
}

// The SUBJECT of a selector: the compound the rule actually paints, which is
// the last one. `.palette-row .type` styles the chip, not the row — reading
// its first compound instead put every `.palette-row` descendant inside a
// chip it is only a sibling of.
export function subjectOf(selector: string): string {
  return compounds(selector).at(-1) ?? "";
}

export function subjectClasses(selector: string): Set<string> {
  return classesOf(subjectOf(selector));
}

// The custom properties a rule's body sets as its INK.
//
// `(?:^|[;{\s])` is what keeps this off --*-color: the character before a
// longhand's `color:` is always a hyphen.
export function inks(body: string): string[] {
  return [...body.matchAll(/(?:^|[;{\s])color:\s*var\((--[\w-]+)\)/g)].map(
    ([, ink]) => ink,
  );
}
