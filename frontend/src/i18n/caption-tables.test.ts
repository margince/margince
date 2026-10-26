// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { reachedModules, SRC_ROOT } from "../../scripts/lib/bundle-reach";
import { parseSource } from "../../scripts/lib/source-tree";
import { LOCALES } from "./index";

// A table of English sentences outside the catalogs reaches every reader in
// English, whatever language they chose. Its captions belong in en, de and vi.

// A capital or a placeholder, then at least one lowercase word: "Loading
// pipeline…" and "{name} is typing" are sentences, "Microsoft Teams" is not.
const WORD = String.raw`[A-Za-z%{][\w%'’{}-]*[,;:]?`;
const LOWER = String.raw`[a-z]{3,}[\w'’-]*[,;:]?`;
const SENTENCE = new RegExp(
  String.raw`^(?:[A-Z][a-z]+[,;:]?|\{\w+\})(?: ${WORD})*(?: ${LOWER})(?: ${WORD})*[.…!?]?$`,
);
// One word is a caption when it says what is going on: "Thinking", "Failed".
// A one-word noun is not caught, since a wire name or a product has its shape.
const STATE_WORD = /^[A-Z][a-z]+(?:ing|ed)[.…]?$/;

function unwrapped(node: ts.Expression): ts.Expression {
  return ts.isAsExpression(node) || ts.isSatisfiesExpression(node)
    ? unwrapped(node.expression)
    : node;
}

/** The sentences a module's top-level object and array tables hold. */
function captionsIn(path: string, text: string): string[] {
  const found: string[] = [];
  const collect = (node: ts.Node): void => {
    if (
      ts.isStringLiteralLike(node) &&
      (SENTENCE.test(node.text) || STATE_WORD.test(node.text))
    ) {
      found.push(node.text);
    }
    ts.forEachChild(node, collect);
  };
  for (const statement of parseSource(path, text).statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (declaration.initializer === undefined) continue;
      const table = unwrapped(declaration.initializer);
      if (
        ts.isObjectLiteralExpression(table) ||
        ts.isArrayLiteralExpression(table)
      ) {
        collect(table);
      }
    }
  }
  return found;
}

describe("the caption-table finder, over planted modules", () => {
  it("finds an English caption in a table", () => {
    expect(
      captionsIn(
        "/app/src/copy.ts",
        `export const SAID = { deals: "Loading pipeline", one: "Loading %s deal" };`,
      ),
    ).toEqual(["Loading pipeline", "Loading %s deal"]);
  });

  it("finds one in a typed array", () => {
    expect(
      captionsIn(
        "/app/src/copy.ts",
        `const WROTE = [["Editing {name}", "Editing deal"]] as const;`,
      ),
    ).toEqual(["Editing deal"]);
  });

  it("finds a caption led by a placeholder, a one-word state and a trailing ellipsis", () => {
    expect(
      captionsIn(
        "/app/src/copy.ts",
        `const SAID = { typing: "{name} is typing", busy: "Thinking", wait: "Loading deals…", end: "Failed." };`,
      ),
    ).toEqual(["{name} is typing", "Thinking", "Loading deals…", "Failed."]);
  });

  it("passes catalog keys, product names and text inside a function", () => {
    expect(
      captionsIn(
        "/app/src/copy.ts",
        [
          `const SAID = { deals: "agent.ticker.said.deals" };`,
          `const NAMES = { meet: "Google Meet", teams: "Microsoft Teams", crm: "Salesforce" };`,
          `const KEYS = { field: "Title", path: "{base}/deals", count: "{count}" };`,
          `function f() { return { x: "Loading pipeline" }; }`,
        ].join("\n"),
      ),
    ).toEqual([]);
  });
});

describe("tables the shipped bundle loads", () => {
  const catalogs = new Set(
    LOCALES.map((locale) => join(SRC_ROOT, "i18n", `${locale}.ts`)),
  );
  const shipped = [...reachedModules({ catalogs }).keys()].filter(
    (path) => !catalogs.has(path),
  );

  // A census that read a smaller tree would pass by reading nothing.
  it("reads the whole bundle, the rail's own tables included", () => {
    expect(shipped.length).toBeGreaterThan(1000);
    expect(shipped).toContain(join(SRC_ROOT, "app", "agentrail-copy.ts"));
  });

  it("hold no English caption outside the catalogs", () => {
    const offenders = shipped.flatMap((path) =>
      captionsIn(path, readFileSync(path, "utf8")).map(
        (caption) => `${relative(SRC_ROOT, path)}: ${caption}`,
      ),
    );
    expect(offenders).toEqual([]);
  });
});
