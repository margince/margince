// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The tree every census rule reads, walked, read and parsed once per run.
// Vitest gives each test file its own module graph, so a parse is shared only
// inside one file. census.test.ts builds this tree and hands it to every rule.

import { execFileSync } from "node:child_process";
import { readdirSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import type ts from "typescript";
import {
  extensionFilesMatching,
  filesMatching,
  MODULE_FILE,
  parseSource,
  sourceTextAt,
} from "../scripts/lib/source-tree";

export const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
export const repoRoot = join(frontendRoot, "..");
export const srcDir = join(frontendRoot, "src");
export const scriptsDir = join(frontendRoot, "scripts");
export const extensionsDir = join(repoRoot, "extensions");

// Modules and stylesheets: what the census rules read today.
const TREE_FILE = /\.(ts|tsx|mts|cts|js|jsx|mjs|cjs|css)$/;

/** Where a file sits: the core source, the build scripts, an extension
 * layer, or the top of frontend/ (the vite and vitest configs). */
export type Area = "src" | "scripts" | "extensions" | "root";

export type CensusFile = Readonly<{
  path: string;
  /** The path from frontend/, with `/`, as findings print it. */
  rel: string;
  area: Area;
  text: string;
  /** The syntax tree of a module; undefined for a stylesheet. */
  source: ts.SourceFile | undefined;
}>;

export type CensusTree = Readonly<{
  files: readonly CensusFile[];
  at(path: string): CensusFile | undefined;
}>;

type RuleBase = Readonly<{
  name: string;
  corpus(tree: CensusTree): readonly CensusFile[];
  /** The floors this rule states beyond a non-empty corpus, each as the
   * sentence to print when it does not hold. */
  floors?(corpus: readonly CensusFile[], tree: CensusTree): readonly string[];
  /** What to do about a finding, printed above the findings. */
  advice: string;
}>;

/**
 * A census rule. Most judge one file at a time, in `visit`. A rule that needs
 * the whole corpus at once judges it in `across`: dialogrows follows a
 * component into the files it renders.
 */
export type CensusRule = RuleBase &
  (
    | Readonly<{ visit(file: CensusFile, tree: CensusTree): readonly string[] }>
    | Readonly<{
        across(
          corpus: readonly CensusFile[],
          tree: CensusTree,
        ): readonly string[];
      }>
  );

function walk(): { path: string; area: Area }[] {
  const top = readdirSync(frontendRoot, { withFileTypes: true })
    .filter((entry) => entry.isFile() && TREE_FILE.test(entry.name))
    .map((entry) => join(frontendRoot, entry.name));
  const inArea = (area: Area, paths: string[]) =>
    paths.map((path) => ({ path, area }));
  return [
    ...inArea("src", filesMatching(srcDir, TREE_FILE)),
    ...inArea("scripts", filesMatching(scriptsDir, TREE_FILE)),
    ...inArea("extensions", extensionFilesMatching(extensionsDir, TREE_FILE)),
    ...inArea("root", top),
  ];
}

/** A tree over the given files: the walked tree, or a rule's planted one. */
export function treeOf(
  entries: readonly { path: string; area: Area; text: string }[],
): CensusTree {
  const files = entries.map(
    ({ path, area, text }): CensusFile => ({
      path,
      rel: relative(frontendRoot, path).split(sep).join("/"),
      area,
      text,
      source: MODULE_FILE.test(path) ? parseSource(path, text) : undefined,
    }),
  );
  const byPath = new Map(files.map((file) => [file.path, file]));
  return { files, at: (path) => byPath.get(path) };
}

export function buildTree(): CensusTree {
  return treeOf(
    walk().map(({ path, area }) => ({ path, area, text: sourceTextAt(path) })),
  );
}

/** What a rule finds over its corpus, by `visit` or `across`. */
export function ruleFindings(
  rule: CensusRule,
  corpus: readonly CensusFile[],
  tree: CensusTree,
): readonly string[] {
  if ("visit" in rule) return corpus.flatMap((file) => rule.visit(file, tree));
  return rule.across(corpus, tree);
}

/** A rule run over a planted tree, the way the runner runs it. */
export const plantedFindings = (
  rule: CensusRule,
  planted: readonly { path: string; area: Area; text: string }[],
): readonly string[] => {
  const tree = treeOf(planted);
  return ruleFindings(rule, rule.corpus(tree), tree);
};

/**
 * The files git knows under the same roots, by the same pattern, from the
 * repository root: what the walk must equal. Untracked files that are not
 * ignored count, so a file not yet added does not fail a local run.
 */
export function gitListedFiles(): string[] {
  const listed = execFileSync(
    "git",
    [
      "ls-files",
      "--cached",
      "--others",
      "--exclude-standard",
      "--",
      "frontend",
      "extensions",
    ],
    { cwd: repoRoot, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 },
  );
  return listed
    .split("\n")
    .filter((path) => TREE_FILE.test(path) && inTreeRoots(path));
}

// The roots of walk(), spelled as paths from the repository root.
function inTreeRoots(path: string): boolean {
  if (/^frontend\/(src|scripts)\//.test(path)) return true;
  if (/^frontend\/[^/]+$/.test(path)) return true;
  const layer = /^extensions\/(?:.+\/)?frontend\/(.+)$/.exec(path);
  if (!layer?.[1]) return false;
  const inside = layer[1].split("/");
  return !inside.includes("node_modules") && !inside.includes("dist");
}
