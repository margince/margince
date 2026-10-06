// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  parseSource,
} from "../../scripts/lib/source-tree";

// One module says how a filter operator reads: filtersentence.ts. A second map
// from an operator to its words is a second answer to "what does gt say on a
// date", and the two drift until a Live List's verdict and the builder's picker
// name one clause differently. So outside that module no shipped source may
// hold a string literal naming a `filters.op.` message.
//
// The census is the tree: every module under src/ and in every extension's
// frontend layer, which ships in the same bundle and speaks the same catalog,
// so a screen written tomorrow is in scope the day it lands.

const REPO = join(import.meta.dirname, "..", "..", "..");
const SRC = join(REPO, "frontend", "src");
const EXTENSIONS = join(REPO, "extensions");
const OWNER = "frontend/src/screens/filtersentence.ts";
const OPERATOR_WORDS = "filters.op.";

// The analytics question engine has operators of its own (`ne`, `is_null`,
// `is_not_null`) that no filter tree carries, so it words them itself, in
// the filter catalog's words.
const EXEMPT: ReadonlyMap<string, string> = new Map([
  [
    "frontend/src/screens/analytics.questions.vocab.ts",
    "maps the analytics engine's own operator enum, not FilterOp",
  ],
]);

// Tests, stories and test kits render nothing the reader sees, and the
// catalogs are where the words are defined.
function shipped(path: string): boolean {
  return (
    !/\.(test|stories|testkit)\.[cm]?[jt]sx?$/.test(path) &&
    !path.startsWith("frontend/src/i18n/")
  );
}

/** Every shipped module, by its path from the repository root. */
function shippedModules(): string[] {
  return [...filesUnder(SRC), ...extensionFrontendFiles(EXTENSIONS)]
    .map((file) => relative(REPO, file))
    .filter(shipped);
}

/** Every string literal in this source that names an operator's message. */
function operatorLiterals(path: string, source: string): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) &&
      node.text.startsWith(OPERATOR_WORDS)
    ) {
      found.push(node.text);
    }
    // A key composed at run time still names the family by its head.
    if (
      ts.isTemplateExpression(node) &&
      node.head.text.startsWith(OPERATOR_WORDS)
    ) {
      found.push(node.head.text);
    }
    ts.forEachChild(node, visit);
  };
  visit(parseSource(path, source));
  return found;
}

function census(modules: readonly string[]): Map<string, string[]> {
  const writers = new Map<string, string[]>();
  for (const path of modules) {
    const literals = operatorLiterals(
      path,
      readFileSync(join(REPO, path), "utf8"),
    );
    if (literals.length > 0) {
      writers.set(path, literals);
    }
  }
  return writers;
}

describe("one module says how a filter operator reads", () => {
  const modules = shippedModules();
  const writers = census(modules);

  it("finds the owner, which proves the scan reaches it", () => {
    expect(writers.get(OWNER)).toContain("filters.op.eq");
  });

  it("reads the extension tier too, which ships in the same bundle", () => {
    expect(
      modules.filter((path) => path.startsWith("extensions/")),
      "the walk reached no extension frontend module",
    ).not.toEqual([]);
  });

  it("finds no other module spelling an operator's words", () => {
    const strays = [...writers.keys()].filter(
      (path) => path !== OWNER && !EXEMPT.has(path),
    );
    expect(
      strays,
      `${strays.join(", ")} spell a filters.op. message: say the clause ` +
        `through ${OWNER} (operatorKey, clauseWords, filterSentence) instead`,
    ).toEqual([]);
  });

  it.each([...EXEMPT.keys()])(
    "still needs the exemption it was given: %s",
    (path) => {
      expect(
        writers.has(path),
        `${path} no longer spells a filters.op. message, so its exemption ` +
          `(${EXEMPT.get(path)}) is stale and must go`,
      ).toBe(true);
    },
  );

  it("reports a planted writer in each shape it reads", () => {
    const planted = [
      `const label = t("filters.op.eq");`,
      // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
      "const label = t(`filters.op.${op}`);",
      "const KEYS = { gt: `filters.op.moreThan` };",
    ].join("\n");
    expect(operatorLiterals("planted.tsx", planted)).toEqual([
      "filters.op.eq",
      "filters.op.",
      "filters.op.moreThan",
    ]);
    expect(
      operatorLiterals("planted.tsx", `// t("filters.op.eq")\nt("x");`),
    ).toEqual([]);
  });

  it("leaves tests, stories, kits and the catalogs out of the census", () => {
    expect(shipped("frontend/src/screens/filters.test.tsx")).toBe(false);
    expect(shipped("frontend/src/screens/filters.stories.tsx")).toBe(false);
    expect(shipped("frontend/src/screens/filters.testkit.ts")).toBe(false);
    expect(shipped("frontend/src/i18n/en.ts")).toBe(false);
    expect(shipped("frontend/src/screens/listexplain.tsx")).toBe(true);
    expect(shipped("extensions/openchannel/frontend/screen.tsx")).toBe(true);
  });
});
