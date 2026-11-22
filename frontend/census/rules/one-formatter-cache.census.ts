// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// An Intl formatter is built only in `src/format/formatters.ts`, which keeps
// each one. A list page asks the same question per row.
// Formatter names come from `intlFormatters()`. Each `toLocale*String` call
// builds a formatter, so it counts as one.
// Tests and stories may build their own. They show nothing to a reader.

import ts from "typescript";
import { stable } from "../../src/format/collate";
import type { CensusFile, CensusRule } from "../tree";
import {
  formatterReached,
  intlAliases,
  intlFormatters,
  isZoneLookup,
  localeMethods,
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

/**
 * The locale methods that format, read from the TypeScript lib declarations:
 * `toLocaleString`, `toLocaleDateString`, `toLocaleTimeString`. Casing and
 * collation take a locale too, but build no formatter.
 */
export function formattingMethods(): string[] {
  return [...localeMethods().keys()]
    .filter((name) => /^toLocale\w*String$/.test(name))
    .sort(stable);
}

// The method a call names, by either spelling.
function methodCalled(callee: ts.Expression): string | undefined {
  if (ts.isPropertyAccessExpression(callee)) return callee.name.text;
  return ts.isElementAccessExpression(callee) &&
    ts.isStringLiteralLike(callee.argumentExpression)
    ? callee.argumentExpression.text
    : undefined;
}

/** Each Intl formatter a file constructs or calls, as `line: Intl.Name`. */
export function constructionsIn(
  parsed: ts.SourceFile,
  formatters: readonly string[],
  methods: readonly string[] = formattingMethods(),
): string[] {
  const bindings = intlAliases(parsed, formatters);
  const found: string[] = [];
  const at = (node: ts.Node, text: string) => {
    const { line } = parsed.getLineAndCharacterOfPosition(
      node.getStart(parsed),
    );
    found.push(`${line + 1}: ${text}`);
  };
  const walk = (node: ts.Node): void => {
    if (ts.isNewExpression(node) || ts.isCallExpression(node)) {
      const formatter = formatterReached(node.expression, formatters, bindings);
      if (formatter !== undefined && !isZoneLookup(node)) {
        at(node, `Intl.${formatter}`);
      }
      const method = methodCalled(node.expression);
      if (method !== undefined && methods.includes(method)) {
        at(node, `.${method}()`);
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
  floors: (_corpus, tree) => {
    const methods = formattingMethods();
    return [
      ...(tree.files.some((file) => file.rel === OWNER)
        ? []
        : [`${OWNER} was not read`]),
      ...["NumberFormat", "DateTimeFormat", "ListFormat"]
        .filter((name) => !intlFormatters().includes(name))
        .map((name) => `the Intl formatters derived lack ${name}`),
      ...["toLocaleDateString", "toLocaleString", "toLocaleTimeString"]
        .filter((name) => !methods.includes(name))
        .map((name) => `the formatting methods derived lack ${name}`),
    ];
  },
  across: (corpus) => {
    const formatters = intlFormatters();
    const methods = formattingMethods();
    const allowed = new Set(ownFormatters.map(({ file }) => file));
    const reached = new Set<string>();
    const findings: string[] = [];
    for (const file of corpus) {
      if (file.source === undefined) continue;
      const found = constructionsIn(file.source, formatters, methods);
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
    "through zonelessDateTimeFormat, which builds it anew each call. " +
    "A toLocaleString call becomes a format helper or numberFormat(...).format.",
};
