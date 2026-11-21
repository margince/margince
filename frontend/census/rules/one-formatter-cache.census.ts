// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// An Intl formatter is built in `src/format/formatters.ts` and nowhere else.
// Building one costs far more than a format() call, and a list page renders
// the same question per row; the cache there builds each formatter once.
//
// The formatter names come from `intlFormatters()`, so a new Intl kind is
// covered without a list here. Tests and stories may build their own: they
// render nothing a reader sees, and some build one to compare against.

import ts from "typescript";
import { stable } from "../../src/format/collate";
import type { CensusFile, CensusRule } from "../tree";
import {
  formatterReached,
  intlAliases,
  intlFormatters,
  isZoneLookup,
} from "./one-locale.census";

const OWNER = "src/format/formatters.ts";

// Files that may build their own formatter, each with its reason.
const ownFormatters: { file: string; why: string }[] = [
  {
    file: "src/mcp-apps/values.ts",
    why: "An MCP app document is a separate bundle inlined into a host page, formatting in the host browser's locale. It has no app Locale to key the cache by.",
  },
];

const NOT_SHIPPED = /\.(test|stories)\.tsx?$/;

const judged = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  file.rel !== OWNER &&
  !NOT_SHIPPED.test(file.rel);

/** Each Intl formatter a file constructs or calls, as `line: Intl.Name`. */
export function constructionsIn(
  parsed: ts.SourceFile,
  formatters: readonly string[],
): string[] {
  const aliases = intlAliases(parsed, formatters);
  const found: string[] = [];
  const walk = (node: ts.Node): void => {
    if (ts.isNewExpression(node) || ts.isCallExpression(node)) {
      const formatter = formatterReached(node.expression, formatters, aliases);
      if (formatter !== undefined && !isZoneLookup(node)) {
        const { line } = parsed.getLineAndCharacterOfPosition(
          node.getStart(parsed),
        );
        found.push(`${line + 1}: Intl.${formatter}`);
      }
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return found;
}

export const rule: CensusRule = {
  name: "one-formatter-cache",
  corpus: (tree) => tree.files.filter(judged),
  floors: (_corpus, tree) => [
    ...(tree.files.some((file) => file.rel === OWNER)
      ? []
      : [`${OWNER} was not read`]),
    ...["NumberFormat", "DateTimeFormat", "ListFormat"]
      .filter((name) => !intlFormatters().includes(name))
      .map((name) => `the Intl formatters derived lack ${name}`),
  ],
  across: (corpus) => {
    const formatters = intlFormatters();
    const allowed = new Set(ownFormatters.map(({ file }) => file));
    const reached = new Set<string>();
    const findings: string[] = [];
    for (const file of corpus) {
      if (file.source === undefined) continue;
      const found = constructionsIn(file.source, formatters);
      if (found.length > 0) reached.add(file.rel);
      if (allowed.has(file.rel)) continue;
      findings.push(...found.map((text) => `${file.rel}:${text}`));
    }
    return [
      ...findings.sort(stable),
      ...ownFormatters
        .filter(({ file }) => !reached.has(file))
        .map(({ file }) => `${file}: allowed, but builds no formatter`),
    ];
  },
  advice:
    "Ask src/format/formatters.ts for the formatter (numberFormat, " +
    "dateTimeFormat, listFormat and the rest), or use a helper in " +
    "src/format/ that already does. A date formatter with no zone goes " +
    "through zonelessDateTimeFormat, which builds it anew each call.",
};
