// Reading CSS as rules and declarations, for the gates that judge one.
//
// It is here rather than inside a *.test.* file because two gates ask the same
// question of the same tree — `onecard.test.ts` about the card surface,
// `oneeyebrow.test.ts` about uppercase micro-type — and a second parser is a
// second answer to "what is a rule", which drifts the way every other pair in
// this tree drifted. It is also not a test file for the reason
// `src/testing/steppedclock.tsx` is not: the design-system and lint gates skip
// test files, and a helper this many gates lean on should answer to the same
// rules the app does.
//
// It is a reader, not a CSS engine. It does not resolve the cascade, compute
// specificity or expand shorthands — a gate that needed those would be asking a
// different question, and each caller expands the equivalences its own subject
// can be written in.

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

/** One `selector { … }` rule, as written. */
export type Rule = Readonly<{ selector: string; body: string }>;

/** Every stylesheet under a directory, node_modules and dist excepted. */
export function stylesheets(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : stylesheets(path);
    }
    return entry.name.endsWith(".css") ? [path] : [];
  });
}

/**
 * CSS comments are not rules, and one sitting above a rule was being read as
 * its selector.
 */
export function withoutComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

export function rules(css: string): Rule[] {
  const found: Rule[] = [];
  // Non-greedy to the first `}`, which is why a nested block (a media query,
  // `:is()` with braces) is not matched as one rule — the rules INSIDE it are,
  // which is what these gates ask about anyway.
  for (const match of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    found.push({ selector: match[1].trim(), body: match[2] });
  }
  return found;
}

/**
 * The `property: value` pairs in a rule body, in one spelling.
 *
 * Every whitespace difference is erased, because each of them computes
 * identically and compares unequal: `background:var(--bgElevated)` without the
 * space, and `var(--bgElevated )` with one inside the parentheses, each let an
 * identical rule escape a gate comparing declaration text. These exist for the
 * hand that does not run the formatter.
 */
export function declarations(body: string): readonly string[] {
  return body
    .split(";")
    .map((part) =>
      part
        .trim()
        .replace(/\s+/g, " ")
        .replace(/\s*:\s*/g, ": ")
        .replace(/\(\s+/g, "(")
        .replace(/\s+\)/g, ")"),
    )
    .filter((part) => part.length > 0);
}

/**
 * The declarations of the ONE rule with this selector in a stylesheet.
 *
 * Exactly one, not the first. `.find()` silently adopts whichever comes first,
 * so a second rule with the same selector anywhere in the file — a media-query
 * override, a leftover — becomes the subject and the gate goes looking for the
 * wrong declarations. With a rare declaration in the first rule that degrades
 * to green while blind, which is the direction a gate must not fail in.
 */
export function soleRule(path: string, selector: string): Rule {
  const declared = rules(withoutComments(readFileSync(path, "utf8"))).filter(
    (rule) => rule.selector === selector,
  );
  if (declared.length !== 1) {
    throw new Error(
      `${path} declares ${selector} ${declared.length} times; a gate needs exactly one to read its subject from`,
    );
  }
  return declared[0];
}
