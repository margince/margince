// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The tree every census rule reads, walked, read and parsed once per run.
// Vitest gives each test file its own module graph, so a parse is shared only
// inside one file. census.test.ts builds this tree and hands it to every rule.

import { execFileSync } from "node:child_process";
import { readdirSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { type Rule, rulesOfText } from "../scripts/lib/css-rules";
import {
  appCompilerOptions,
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
export const e2eDir = join(frontendRoot, "e2e");
export const storybookDir = join(frontendRoot, ".storybook");

// Modules, stylesheets, and the HTML and MDX pages a bundler or Storybook reads.
const TREE_FILE = /\.(ts|tsx|mts|cts|js|jsx|mjs|cjs|css|html|mdx)$/;

/** Where a file sits. It is the core source, the build scripts or an
 * extension layer. It can also be the top of frontend/ (the vite and vitest
 * configs, index.html), the Playwright suite, or the Storybook config. */
export type Area =
  | "src"
  | "scripts"
  | "extensions"
  | "root"
  | "e2e"
  | "storybook";

// The areas the shipped app and its build are compiled from. The type-checked
// program covers these and leaves the Playwright suite and Storybook config out.
const COMPILED_AREAS: ReadonlySet<Area> = new Set([
  "src",
  "scripts",
  "extensions",
  "root",
]);

export type CensusFile = Readonly<{
  path: string;
  /** The path from frontend/, with `/`, as findings print it. */
  rel: string;
  area: Area;
  text: string;
  /** The syntax tree of a module; undefined for a stylesheet or a page. */
  source: ts.SourceFile | undefined;
}>;

export type CensusTree = Readonly<{
  files: readonly CensusFile[];
  at(path: string): CensusFile | undefined;
  /**
   * One type-checked program over the compiled modules of the tree. It is
   * built on first ask from the trees already parsed, and shared by every rule
   * that asks.
   */
  checked(): ts.Program;
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
 *
 * A rule whose work outgrows one test's budget judges it in `settle`. The
 * runner starts every `settle` once the tree is built and awaits it after the
 * other rules. So the work runs in a worker thread beside them.
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
    | Readonly<{
        settle(
          corpus: readonly CensusFile[],
          tree: CensusTree,
        ): Promise<readonly string[]>;
      }>
  );

/** A rule whose findings come in its own time, in a worker. */
export type SettledRule = Extract<CensusRule, { settle: unknown }>;

export const settles = (rule: CensusRule): rule is SettledRule =>
  "settle" in rule;

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
    ...inArea("e2e", filesMatching(e2eDir, TREE_FILE)),
    ...inArea("storybook", filesMatching(storybookDir, TREE_FILE)),
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
  const at = (path: string) => byPath.get(path);
  let program: ts.Program | undefined;
  const checked = () => {
    program ??= checkedOver(
      files.filter(
        (file) => file.source !== undefined && COMPILED_AREAS.has(file.area),
      ),
      at,
    );
    return program;
  };
  return { files, at, checked };
}

// The app's own compiler options, so imports resolve as the build resolves
// them. A file the tree parsed is handed over rather than parsed again.
function checkedOver(
  modules: readonly CensusFile[],
  at: (path: string) => CensusFile | undefined,
): ts.Program {
  const options = {
    ...appCompilerOptions(frontendRoot),
    noEmit: true,
    incremental: false,
  };
  const host = ts.createCompilerHost(options, true);
  const read = host.getSourceFile.bind(host);
  host.getSourceFile = (name, version, onError, create) =>
    at(name)?.source ?? read(name, version, onError, create);
  return ts.createProgram(
    modules.map((file) => file.path),
    options,
    host,
  );
}

/** Every sheet the bundle ships: the core's and each extension layer's. */
export const isAppSheet = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.path.endsWith(".css");

/** The style rules of the given sheets, one per selector in a list. They are
 * read from the tree's text the way css-rules.ts reads a sheet from disk. */
export const sheetRules = (sheets: readonly CensusFile[]): Rule[] =>
  sheets.flatMap((sheet) => rulesOfText(sheet.path, sheet.text));

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
  if ("across" in rule) return rule.across(corpus, tree);
  throw new Error(`${rule.name} settles in a worker: await its settle`);
}

/** A rule run over a planted tree, the way the runner runs it. */
export const plantedFindings = (
  rule: CensusRule,
  planted: readonly { path: string; area: Area; text: string }[],
): readonly string[] => {
  const tree = treeOf(planted);
  return ruleFindings(rule, rule.corpus(tree), tree);
};

/** A settled rule run over a planted tree, the way the runner runs it. */
export const plantedSettled = (
  rule: SettledRule,
  planted: readonly { path: string; area: Area; text: string }[],
): Promise<readonly string[]> => {
  const tree = treeOf(planted);
  return rule.settle(rule.corpus(tree), tree);
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
  if (/^frontend\/(src|scripts|e2e|\.storybook)\//.test(path)) return true;
  if (/^frontend\/[^/]+$/.test(path)) return true;
  const layer = /^extensions\/(?:.+\/)?frontend\/(.+)$/.exec(path);
  if (!layer?.[1]) return false;
  const inside = layer[1].split("/");
  return !inside.includes("node_modules") && !inside.includes("dist");
}
