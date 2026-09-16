// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// The sheet every token gate reads, and the two ways of reading it.
//
// It lives here rather than in one of them because there are two now —
// tokens.test.ts pins the Ledger-Green palette, type-tokens.test.ts pins the
// type scale and the control geometry — and a second copy of `parseBlock` is a
// second answer to what a declaration IS. The narrower copy would read fewer
// tokens and report the same word, PASS.

const here = dirname(fileURLToPath(import.meta.url));

/** tokens.css as authored, for anything that reports a line a reader can open. */
export const tokensCss = readFileSync(join(here, "tokens.css"), "utf8");

/**
 * The same sheet with its comments removed, for anything that reads
 * DECLARATIONS. A declaration and a sentence about a declaration are not the
 * same thing, and that file explains most of its values in prose that names
 * them; left in, the prose parses as declarations of its own.
 */
export const tokenDecls = tokensCss.replace(/\/\*[\s\S]*?\*\//g, "");

/**
 * A value with its formatting taken out — case, whitespace and a leading zero
 * before a decimal point — so a comparison is about the value and not about how
 * the formatter last wrapped it.
 */
export function normalize(value: string): string {
  return value
    .toLowerCase()
    .replace(/\s+/g, "")
    .replace(/(^|[^0-9])0\./g, "$1.")
    .replace(/(\.[0-9]*?)0+([^0-9]|$)/g, "$1$2");
}

/** Every custom property one block declares, by name. */
export function parseBlock(
  css: string,
  selector: string,
): Record<string, string> {
  // Parentheses and the colon are escaped too, so a selector carrying a
  // functional pseudo-class (`:root:not([data-theme="light"])`) is matched as
  // the literal text it is rather than compiled into a capture group — which
  // matches nothing, and would report the block as missing.
  const match = css.match(
    new RegExp(`${selector.replace(/[[\]"=():]/g, "\\$&")}\\s*\\{([^}]*)\\}`),
  );
  if (!match) {
    throw new Error(`tokens.css has no ${selector} block`);
  }
  const props: Record<string, string> = {};
  for (const [, name, value] of match[1].matchAll(
    /(--[\w-]+)\s*:\s*([^;]+);/g,
  )) {
    props[name] = value.trim();
  }
  return props;
}
