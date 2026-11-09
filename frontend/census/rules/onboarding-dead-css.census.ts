// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Dead CSS reads as intent. A rule with a comment explaining its layout looks
// live until somebody greps the sources. So the next developer to touch a
// neighbouring rule has to guess whether it matters.
//
// Sheets and classes both come from the tree, so a new onboarding sheet is
// gated the day it lands. Nothing is scoped to a prefix. The one hand-kept
// list is `runtimeClasses`, and it stays short.

import { existsSync, readFileSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import {
  aliases,
  buildScriptEntries,
  moduleGraph,
  type SourceTree,
  surfaceEntries,
} from "../../scripts/lib/bundle-reach";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  frontendRoot,
  srcDir,
} from "../tree";

const onboardingDir = join(srcDir, "features", "onboarding");
const conversationDir = join(onboardingDir, "onboarding-conversation");

const isGatedSheet = (file: CensusFile) =>
  file.path.endsWith(".css") &&
  ((dirname(file.path) === onboardingDir &&
    basename(file.path).startsWith("onboarding")) ||
    dirname(file.path) === conversationDir);

// A class named in prose is no class in use: a story's comment once kept
// `.dropzone` alive. A line comment counts only when it owns its line, so a
// `//` inside a URL cannot eat the code beside it.
export function code(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, " ")
    .replace(/^[ \t]*\/\/.*$/gm, " ");
}

export function declaredClasses(css: string): string[] {
  const rules = css.replace(/\/\*[\s\S]*?\*\//g, "");
  return [
    ...new Set(
      [...rules.matchAll(/\.(-?[A-Za-z_][A-Za-z0-9_-]*)/g)].map(
        (m) => m[1] ?? "",
      ),
    ),
  ];
}

/** The selectors of rules whose body is empty, comments left out. */
export function hollowRules(css: string): string[] {
  const rules = css.replace(/\/\*[\s\S]*?\*\//g, "");
  return [...rules.matchAll(/([^{}]*)\{([^{}]*)\}/g)]
    .filter(([, , body]) => (body ?? "").trim() === "")
    .map(([, selector]) => (selector ?? "").trim());
}

// The module scripts an HTML document loads, as absolute paths.
function scriptsOf(document: string): string[] {
  return [...readFileSync(document, "utf8").matchAll(/<script\b([^>]*)>/g)]
    .map(([, attributes]) => attributes ?? "")
    .filter((attributes) => /\btype="module"/.test(attributes))
    .flatMap((attributes) => [...attributes.matchAll(/\bsrc="([^"]+)"/g)])
    .map(([, source]) =>
      (source ?? "").startsWith("/")
        ? join(frontendRoot, source ?? "")
        : resolve(dirname(document), source ?? ""),
    );
}

// Vite builds the root index.html, and each MCP view from an index.html beside
// its modules. Looking beside each folder of the tree finds them without a walk.
function documentEntries(files: readonly CensusFile[]): string[] {
  const folders = new Set(files.map((file) => dirname(file.path)));
  const documents = [
    join(frontendRoot, "index.html"),
    ...[...folders]
      .map((folder) => join(folder, "index.html"))
      .filter((document) => existsSync(document)),
  ];
  return documents.flatMap(scriptsOf);
}

type Reached = Readonly<{ path: string; code: string }>;

// The modules the bundle loads, not every file under src/. A rule reached only
// by a story or a test is dead to the user.
const reachedByTree = new WeakMap<CensusTree, Reached[]>();
function reachedCode(tree: CensusTree): Reached[] {
  const known = reachedByTree.get(tree);
  if (known) return known;
  const inSrc = tree.files.filter((file) => file.area === "src");
  const sources: SourceTree = new Map(
    inSrc.map((file): [string, string] => [file.path, file.text]),
  );
  const reached = moduleGraph({
    tree: sources,
    aliases: aliases(),
    catalogs: new Set(),
  }).reach([
    ...documentEntries(inSrc),
    ...surfaceEntries(),
    ...buildScriptEntries(),
  ]);
  const result = [...reached.keys()].map((path) => ({
    path,
    code: code(sources.get(path) ?? ""),
  }));
  reachedByTree.set(tree, result);
  return result;
}

// A name composed at run time (`is-${notice.tone}`) is never a whole token.
// Each entry carries its proof: `composedAs` must still be in `composedBy`'s
// code, and each variant must still be spelled there.
const runtimeClasses = [
  {
    composedBy: "onboarding-gate.tsx",
    head: "is-",
    composedAs: `is-\${notice.tone}`,
    variants: ["error", "paused", "resumed"],
  },
  {
    // The triage row styles the shared confidence meter, which composes its
    // modifier from the ConfidenceLevel union.
    composedBy: "trust.tsx",
    head: "confidence-",
    composedAs: `confidence-\${level}`,
    variants: ["high", "med", "low"],
  },
] as const;

const composedAtRuntime = new Set<string>(
  runtimeClasses.flatMap((entry) =>
    entry.variants.map((variant) => `${entry.head}${variant}`),
  ),
);

/** Whole tokens, hyphens included, so `.ob-live` is not vouched for by `.ob-live-card`. */
const tokensOf = (modules: readonly Reached[]) =>
  new Set(
    modules.flatMap(({ code: text }) =>
      [...text.matchAll(/[A-Za-z][A-Za-z0-9_-]*/g)].map((m) => m[0]),
    ),
  );

/** The findings over the gated sheets, given the code the bundle loads. */
export function deadCss(
  sheets: readonly CensusFile[],
  modules: readonly Reached[],
): string[] {
  const named = tokensOf(modules);
  const orphans = sheets.flatMap((sheet) =>
    declaredClasses(sheet.text)
      .filter((name) => !named.has(name) && !composedAtRuntime.has(name))
      .map((name) => `${sheet.rel}: .${name} is named by no shipped source`),
  );
  // Two files sharing a name leave the entry unable to say which composes it.
  const stale = runtimeClasses.flatMap((entry) => {
    const matches = modules.filter(
      ({ path }) => basename(path) === entry.composedBy,
    );
    const only = matches.length === 1 ? matches[0] : undefined;
    if (!only) return [`${entry.composedBy} is not one file under src/`];
    const missing = [
      ...(only.code.includes(entry.composedAs) ? [] : [entry.composedAs]),
      ...entry.variants.filter((v) => !only.code.includes(`"${v}"`)),
    ];
    return missing.map((gone) => `${entry.composedBy} no longer has ${gone}`);
  });
  // A deleted rule body leaves a selector that still parses and paints nothing.
  const hollow = sheets.flatMap((sheet) =>
    hollowRules(sheet.text).map(
      (selector) => `${sheet.rel}: ${selector} has an empty body`,
    ),
  );
  return [...orphans, ...stale, ...hollow];
}

export const rule: CensusRule = {
  name: "onboarding-dead-css",
  corpus: (tree) => tree.files.filter(isGatedSheet),
  // A miswired corpus passes every finding by inspecting nothing.
  floors: (corpus, tree) => {
    const modules = reachedCode(tree);
    return [
      ...(corpus.length > 5 ? [] : ["5 onboarding sheets or fewer were read"]),
      ...(modules.length > 100
        ? []
        : ["the bundle reached 100 modules or fewer"]),
      ...(tokensOf(modules).size > 1000
        ? []
        : ["the shipped sources name 1000 tokens or fewer"]),
    ];
  },
  across: (corpus, tree) => deadCss(corpus, reachedCode(tree)),
  advice:
    "Delete the dead rule, and a selector left with an empty body. If the " +
    "class is composed at run time, add it to runtimeClasses with the " +
    "expression and the file that compose it.",
};
