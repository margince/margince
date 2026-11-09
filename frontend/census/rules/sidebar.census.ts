// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Storybook tree is shelved the way README.md's "Seeing them" tables say,
// so a reader finds a component where the catalog sends them.
//
// 1. Every documented root holds a story, or a docs page for `Get started/`,
//    and the introduction lists the roots in the same order.
// 2. Every `Components/` and `Foundations/` story sits on a declared shelf,
//    and every declared shelf holds one.
// 3. Under every documented root, titles are Sentence case, one per file, and
//    never both a leaf and a group.
// 4. `.storybook/preview.tsx` sorts the sidebar in the catalog's order.
//
// Which roots a story may be filed under at all is the catalog rule's.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { sanitize } from "storybook/internal/csf";
import ts from "typescript";
import { readDesignCatalog } from "../../scripts/lib/design-catalog";
import { parseSource } from "../../scripts/lib/source-tree";
import { isDocsPage, storySuffixes } from "../../scripts/lib/story-files";
import {
  defaultExportObject,
  literalAt,
  storyTitle,
} from "../../scripts/lib/story-title";
import {
  type CensusFile,
  type CensusTree,
  frontendRoot,
  type SettledRule,
  srcDir,
  storybookDir,
} from "../tree";

const previewPath = join(storybookDir, "preview.tsx");
const introductionPath = join(srcDir, "design-system", "introduction.mdx");

// Spellings that hold wherever they stand in a segment, each with its reason.
const PROPER_NOUNS = new Map([
  ["Margince", "the product's name"],
  ["LinkedIn", "the company's name"],
  ["OAuth", "the protocol, as its specification spells it"],
  ["vCard", "the RFC 6350 format, whose name starts lowercase"],
  ["Shortlist", "a product noun the copy capitalises: Add to Shortlist"],
  ["MCP Apps", "the MCP extension's own name, as mcp-apps/bridge.ts writes it"],
]);
const ACRONYMS = new Map([
  ["AI", "the agent tier, as the product's copy writes it"],
  ["IMAP", "the mail protocol, as the copy writes it"],
  ["DNA", "Voice DNA, the product's name for a writing voice"],
  ["VAT", "the tax, as the copy writes it"],
  ["MCP", "the Model Context Protocol, as the copy writes it"],
]);

export type Filed = { path: string; title: string };

function rootOf(title: string): string {
  return title.split("/")[0];
}

// The one root that holds docs pages rather than stories.
const DOCS_ROOT = "Get started";

export function emptyRootFindings(
  filed: Filed[],
  documented: string[],
): string[] {
  return documented.flatMap((root) => {
    const docs = root === DOCS_ROOT;
    const held = filed.some(
      ({ path, title }) => rootOf(title) === root && isDocsPage(path) === docs,
    );
    if (held) return [];
    return [
      `${root}/ is documented and holds no ${docs ? "docs page" : "story"}`,
    ];
  });
}

// The bolded names on the bullets under the introduction's "Where things are".
export function introductionRoots(mdx: string): string[] {
  const lines = mdx.split("\n");
  const start = lines.indexOf("## Where things are");
  if (start < 0) return [];
  const rest = lines.slice(start + 1);
  const end = rest.findIndex((line) => line.startsWith("## "));
  return (end < 0 ? rest : rest.slice(0, end))
    .filter((line) => line.startsWith("- "))
    .flatMap((line) =>
      [...line.matchAll(/\*\*([^*]+)\*\*/g)].map((match) => match[1]),
    );
}

// `deepest` is one level past `shallowest`: a component's sub-node, as Decision
// deck/Frame is, or a topic group's, as Color/Interaction colors is.
export function shelfFindings(
  filed: Filed[],
  root: string,
  shelves: string[],
  [shallowest, deepest]: [number, number],
): string[] {
  const under = filed.filter(({ title }) => rootOf(title) === root);
  const misfiled = under.flatMap(({ path, title }) => {
    const segments = title.split("/");
    return segments.length >= shallowest &&
      segments.length <= deepest &&
      shelves.includes(segments[1])
      ? []
      : [
          `${path} files "${title}" off the ${root}/ shelves the README declares`,
        ];
  });
  const reached = new Set(under.map(({ title }) => title.split("/")[1]));
  const empty = shelves
    .filter((shelf) => !reached.has(shelf))
    .map((shelf) => `${root}/${shelf} is declared and holds no story`);
  return [...misfiled, ...empty];
}

const DECLARED = [...PROPER_NOUNS.keys(), ...ACRONYMS.keys()];
const DECLARED_WORDS = DECLARED.filter((spelling) => !spelling.includes(" "));
const DECLARED_PHRASES = DECLARED.filter((spelling) =>
  spelling.includes(" "),
).map((spelling) => spelling.split(" "));

function sameWords(words: string[], phrase: string[]): boolean {
  return (
    words.length === phrase.length &&
    words.every((word, i) => word.toLowerCase() === phrase[i].toLowerCase())
  );
}

export function isSentenceCase(segment: string): boolean {
  if (!/^\S+( \S+)*$/.test(segment)) return false;
  const words = segment.split(" ");
  let index = 0;
  while (index < words.length) {
    const phrase = DECLARED_PHRASES.find((spelling) =>
      sameWords(words.slice(index, index + spelling.length), spelling),
    );
    if (phrase === undefined) {
      if (!isSentenceWord(words[index], index === 0)) return false;
      index += 1;
      continue;
    }
    const written = words.slice(index, index + phrase.length);
    if (written.join(" ") !== phrase.join(" ")) return false;
    index += phrase.length;
  }
  return true;
}

function isSentenceWord(word: string, first: boolean): boolean {
  const parts = word.split("-").map((part) => part.replace(/['’]s$/u, ""));
  return parts.every((part, index) => {
    const declared = DECLARED_WORDS.find(
      (spelling) => spelling.toLowerCase() === part.toLowerCase(),
    );
    if (declared !== undefined) return part === declared;
    if (/^\p{N}+$/u.test(part)) return true;
    return first && index === 0
      ? /^\p{Lu}[^\p{Lu}]*$/u.test(part)
      : !/\p{Lu}/u.test(part);
  });
}

export function caseFindings(filed: Filed[]): string[] {
  return filed.flatMap(({ path, title }) =>
    title
      .split("/")
      .filter((segment) => !isSentenceCase(segment))
      .map((segment) => `${path}: "${segment}" is not Sentence case`),
  );
}

export function sharedTitleFindings(filed: Filed[]): string[] {
  const byId = new Map<string, string[]>();
  for (const { path, title } of filed) {
    const id = sanitize(title);
    byId.set(id, [...(byId.get(id) ?? []), `${path} ("${title}")`]);
  }
  return [...byId]
    .filter(([, claims]) => claims.length > 1)
    .map(([id, claims]) => `id "${id}" is claimed by ${claims.join(", ")}`);
}

// A group's id is its path's, folded whole, so `A/B` and `A b/C` meet at `a-b`.
export function leafAndGroupFindings(filed: Filed[]): string[] {
  const groups = filed.flatMap(({ title }) => {
    const segments = title.split("/");
    return segments.slice(1).map((_, depth) => ({
      id: sanitize(segments.slice(0, depth + 1).join("/")),
      child: title,
    }));
  });
  return filed.flatMap(({ path, title }) => {
    const id = sanitize(title);
    const group = groups.find((candidate) => candidate.id === id);
    return group === undefined
      ? []
      : [
          `${path}: "${title}" is a leaf and also the group of "${group.child}"`,
        ];
  });
}

export type SidebarOrder = { roots: string[]; children: Map<string, string[]> };

// Storybook reads this literal statically and refuses anything else, and so
// does this reader: strings, each followed by at most one string array.
export function sidebarOrder(path: string, text: string): SidebarOrder | null {
  const order = literalAt(defaultExportObject(parseSource(path, text)), [
    "parameters",
    "options",
    "storySort",
    "order",
  ]);
  if (order === undefined || !ts.isArrayLiteralExpression(order)) return null;
  const sorted: string[] = [];
  const children = new Map<string, string[]>();
  for (const element of order.elements) {
    if (ts.isStringLiteralLike(element)) {
      sorted.push(element.text);
      continue;
    }
    const root = sorted.at(-1);
    if (
      !ts.isArrayLiteralExpression(element) ||
      root === undefined ||
      children.has(root)
    ) {
      return null;
    }
    const names = element.elements.filter(ts.isStringLiteralLike);
    if (names.length !== element.elements.length) return null;
    children.set(
      root,
      names.map((name) => name.text),
    );
  }
  return sorted.length > 0 ? { roots: sorted, children } : null;
}

// Read off Storybook's own globs, so a new kind of story file joins the corpus.
const mainPath = join(storybookDir, "main.ts");
const suffixes = storySuffixes(mainPath, readFileSync(mainPath, "utf8"));

const loadedByStorybook = (file: CensusFile) =>
  file.area === "src" &&
  (suffixes ?? []).some((suffix) => file.path.endsWith(suffix));

const orderIn = (tree: CensusTree) => {
  const preview = tree.at(previewPath);
  return preview ? sidebarOrder(previewPath, preview.text) : null;
};

const introducedIn = (tree: CensusTree) =>
  introductionRoots(tree.at(introductionPath)?.text ?? "");

const listed = (names: readonly string[] | undefined) =>
  (names ?? []).join(", ");

/** What the sidebar holds against the catalog, given every filed title. */
export function sidebarFindings(filed: Filed[], tree: CensusTree): string[] {
  const { roots, categories, topics } = readDesignCatalog(frontendRoot);
  const shaped = filed.filter(({ title }) => roots.includes(rootOf(title)));
  const under = (root: string) =>
    shaped.filter(({ title }) => rootOf(title) === root).length;
  const introduced = introducedIn(tree);
  const order = orderIn(tree);
  const same = (said: readonly string[] | undefined, wanted: string[]) =>
    listed(said) === listed(wanted);
  return [
    ...(shaped.length > 550 ? [] : ["550 shelved titles or fewer were read"]),
    ...(under("Components") > 50
      ? []
      : ["50 Components/ titles or fewer were read"]),
    ...(under("Foundations") >= topics.length
      ? []
      : ["fewer Foundations/ titles than declared topics were read"]),
    ...emptyRootFindings(filed, roots),
    ...(same(introduced, roots)
      ? []
      : [
          `the introduction lists ${listed(introduced)}; the catalog documents ${listed(roots)}`,
        ]),
    ...shelfFindings(filed, "Components", categories, [3, 4]),
    ...shelfFindings(filed, "Foundations", topics, [2, 3]),
    ...caseFindings(shaped),
    ...sharedTitleFindings(shaped),
    ...leafAndGroupFindings(shaped),
    ...(same(order?.roots, roots)
      ? []
      : [`the preview sorts the roots ${listed(order?.roots)}`]),
    ...(same(order?.children.get("Components"), categories)
      ? []
      : [
          `the preview sorts Components/ ${listed(order?.children.get("Components"))}`,
        ]),
    ...(same(order?.children.get("Foundations"), topics)
      ? []
      : [
          `the preview sorts Foundations/ ${listed(order?.children.get("Foundations"))}`,
        ]),
  ];
}

export const rule: SettledRule = {
  name: "sidebar",
  corpus: (tree) => tree.files.filter(loadedByStorybook),
  floors: (_corpus, tree) => {
    const { roots, categories, topics } = readDesignCatalog(frontendRoot);
    return [
      ...(suffixes === null
        ? ["the story globs in .storybook/main.ts could not be read"]
        : []),
      ...(roots.length > 4
        ? []
        : ["the catalog documents four roots or fewer"]),
      ...(categories.length > 5
        ? []
        : ["the catalog declares five categories or fewer"]),
      ...(topics.length > 1 ? [] : ["the catalog declares one topic or none"]),
      ...(orderIn(tree) === null
        ? ["no sidebar order could be read off .storybook/preview.tsx"]
        : []),
      ...(introducedIn(tree).length > 4
        ? []
        : ["the introduction lists four roots or fewer"]),
    ];
  },
  // Storybook's MDX analyser is async, so the titles settle after the
  // synchronous rules.
  settle: async (corpus, tree) => {
    const titled = await Promise.all(
      corpus.map(async (file) => ({
        path: file.rel,
        title: await storyTitle(file.path, file.text),
      })),
    );
    return sidebarFindings(
      titled.flatMap(({ path, title }) =>
        title === null ? [] : [{ path, title }],
      ),
      tree,
    );
  },
  advice:
    "Shelve every story the way src/design-system/README.md's tables say: a " +
    "documented root, a declared category or topic, Sentence case, one title " +
    "per file, and the same order in .storybook/preview.tsx and the introduction.",
};
