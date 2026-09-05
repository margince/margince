// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// The settings catalog is split in two so the shell can ask where a settings
// entry lives without paying to draw it. `settingsnav.tsx` answers the address
// and visibility questions; `settings.tsx` renders the cards and pulls in every
// mutation hook and roughly a hundred and fifty imports to do it.
//
// That split only buys anything while it holds TRANSITIVELY. A direct-import
// grep would pass the day someone adds `settingsnav -> somecard -> settings`,
// and the shell would silently go back to loading the whole settings screen to
// render a nav rail. So this walks the graph from each entry point and fails on
// the reachable set, not on the first hop.
//
// Two obligations, both real failures we would otherwise ship blind:
//
//  1. No file under `src/app/**` may reach `settings.tsx`. Those are the always-
//     loaded shell modules; reaching the screen defeats its lazy chunk for every
//     page in the product, not only settings.
//  2. `settingsnav.tsx` may not reach `settings.tsx`. It is the light half by
//     construction, and a cycle through it re-couples every one of its readers.

const screensDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(screensDir, "..");
const settingsScreen = join(screensDir, "settings.tsx");
const settingsNav = join(screensDir, "settingsnav.tsx");

/** Resolve a relative specifier to a real file, trying the extensions Vite does. */
function resolveSpecifier(fromFile: string, specifier: string): string | null {
  if (!specifier.startsWith(".")) {
    return null; // a package, not our source
  }
  const base = resolve(dirname(fromFile), specifier);
  for (const candidate of [
    base,
    `${base}.ts`,
    `${base}.tsx`,
    join(base, "index.ts"),
    join(base, "index.tsx"),
  ]) {
    // `isFile` rather than mere existence: a bare specifier can name a
    // DIRECTORY that also has an `index.tsx`, and treating the directory as the
    // resolved module would end the walk one hop early — a silent under-read,
    // which is the one way this gate must not fail.
    if (existsSync(candidate) && statSync(candidate).isFile()) {
      return candidate;
    }
  }
  return null;
}

/**
 * Every module specifier `file` imports, including `import type` and dynamic
 * `import()`. Type-only edges count: a type import that names a card module
 * still says the two halves are one unit, and a later value import across the
 * same edge would not show up as a new dependency in review.
 */
function importsOf(file: string): string[] {
  const source = ts.createSourceFile(
    file,
    readFileSync(file, "utf8"),
    ts.ScriptTarget.Latest,
    true,
  );
  const out: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      out.push(node.moduleSpecifier.text);
    }
    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments.length > 0 &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      out.push(node.arguments[0].text);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

/**
 * The shortest import path from `entry` to `target`, or null when unreachable.
 * Returning the path rather than a boolean is what makes a failure actionable:
 * the offending edge is usually three hops in and invisible from the entry.
 */
function pathTo(entry: string, target: string): string[] | null {
  const seen = new Set([entry]);
  const queue: string[][] = [[entry]];
  while (queue.length > 0) {
    const trail = queue.shift() as string[];
    const head = trail[trail.length - 1];
    for (const specifier of importsOf(head)) {
      const next = resolveSpecifier(head, specifier);
      if (next === null || seen.has(next)) {
        continue;
      }
      if (next === target) {
        return [...trail, next].map((file) => relative(srcRoot, file));
      }
      seen.add(next);
      queue.push([...trail, next]);
    }
  }
  return null;
}

/**
 * Every production module that must not reach the settings screen, DERIVED
 * rather than listed. A hand-kept list is a second copy of "who asks a settings
 * question", and it goes stale the first time somebody adds a module — silently,
 * because a gate that reads a smaller tree still reports PASS.
 *
 * Two trees qualify. `src/app/**` is the always-loaded shell. `src/screens/**`
 * is every other screen: a screen that only wants an ADDRESS must not drag the
 * settings cards into its own chunk, which is what `worklist.copy.ts` did —
 * putting the whole settings screen behind Home, the default landing page.
 *
 * Tests, stories and the testkit are excluded on purpose: they ask this module
 * for both halves, that is what they are for, and they ship in no chunk.
 */
function productionModulesUnder(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return productionModulesUnder(path);
    }
    if (!/\.tsx?$/.test(entry.name)) {
      return [];
    }
    return /\.(test|stories)\.tsx?$|\.testkit\.tsx$/.test(entry.name)
      ? []
      : [path];
  });
}

const settingsOwnModules = new Set([settingsScreen, settingsNav]);
const shellEntryPoints = [
  ...productionModulesUnder(join(srcRoot, "app")),
  ...productionModulesUnder(join(srcRoot, "screens")),
]
  .filter((file) => !settingsOwnModules.has(file))
  .map((file) => relative(srcRoot, file))
  .sort();

describe("the settings nav split holds transitively", () => {
  it.each(shellEntryPoints)("%s does not reach settings.tsx", (entry) => {
    const trail = pathTo(join(srcRoot, entry), settingsScreen);
    expect(
      trail,
      trail === null ? "" : `import path: ${trail.join(" -> ")}`,
    ).toBeNull();
  });

  it("settingsnav.tsx does not reach settings.tsx", () => {
    const trail = pathTo(settingsNav, settingsScreen);
    expect(
      trail,
      trail === null ? "" : `import path: ${trail.join(" -> ")}`,
    ).toBeNull();
  });

  // The walk is only worth its runtime if it can actually see an edge. Without
  // this, a resolver that silently returned null for every specifier would
  // report PASS on a tree where the split had completely collapsed.
  it("finds a path that does exist, so a green result means something", () => {
    const trail = pathTo(settingsScreen, join(srcRoot, "i18n/index.tsx"));
    expect(trail).not.toBeNull();
    expect(trail?.[0]).toBe("screens/settings.tsx");
  });
});
