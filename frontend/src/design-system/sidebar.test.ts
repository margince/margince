// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Sidebar gate: the Storybook tree is shelved the way README.md's "Seeing
// them" tables say, so a reader finds a component where the catalog sends them.
//
// ## Arms, one subject
//
// 1. Every root the catalog documents holds a story, or a docs page for
//    `Get started/`, and the introduction lists the roots in the same order.
// 2. Every `Components/` and `Foundations/` story sits on a shelf the catalog
//    declares, and every declared shelf holds one.
// 3. Under every documented root, titles are Sentence case, one per file, and never
//    both a leaf and a group.
// 4. `.storybook/preview.tsx` sorts the sidebar in the catalog's order.
//
// Which roots a story may be filed under at all is catalog.test.ts's.

import { readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { sanitize } from "storybook/internal/csf";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { readDesignCatalog } from "../../scripts/lib/design-catalog";
import { parseSource } from "../../scripts/lib/source-tree";
import { isDocsPage, storyCensus } from "../../scripts/lib/story-files";
import {
  defaultExportObject,
  literalAt,
  titledStories,
} from "../../scripts/lib/story-title";

const frontendRoot = resolve(__dirname, "..", "..");
const previewPath = join(frontendRoot, ".storybook", "preview.tsx");
const introductionPath = join(
  frontendRoot,
  "src",
  "design-system",
  "introduction.mdx",
);

const { roots, categories, topics } = readDesignCatalog(frontendRoot);

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

type Filed = { path: string; title: string };

function rootOf(title: string): string {
  return title.split("/")[0];
}

// The one root that holds docs pages rather than stories.
const DOCS_ROOT = "Get started";

function emptyRootFindings(filed: Filed[], documented: string[]): string[] {
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
function introductionRoots(mdx: string): string[] {
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
function shelfFindings(
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

function isSentenceCase(segment: string): boolean {
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

function caseFindings(filed: Filed[]): string[] {
  return filed.flatMap(({ path, title }) =>
    title
      .split("/")
      .filter((segment) => !isSentenceCase(segment))
      .map((segment) => `${path}: "${segment}" is not Sentence case`),
  );
}

function sharedTitleFindings(filed: Filed[]): string[] {
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
function leafAndGroupFindings(filed: Filed[]): string[] {
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

type SidebarOrder = { roots: string[]; children: Map<string, string[]> };

// Storybook reads this literal statically and refuses anything else, and so
// does this reader: strings, each followed by at most one string array.
function sidebarOrder(path: string, text: string): SidebarOrder | null {
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

const filed = (await titledStories(storyCensus(frontendRoot).files)).flatMap(
  ({ path, title }) =>
    title === null ? [] : [{ path: relative(frontendRoot, path), title }],
);

describe("the sidebar is shelved the way the catalog says", () => {
  const introduced = introductionRoots(readFileSync(introductionPath, "utf8"));
  const shaped = filed.filter(({ title }) => roots.includes(rootOf(title)));
  const order = sidebarOrder(previewPath, readFileSync(previewPath, "utf8"));

  it("reads the shelves and titles it holds", () => {
    expect(roots.length).toBeGreaterThan(4);
    expect(categories.length).toBeGreaterThan(5);
    expect(topics.length).toBeGreaterThan(1);
    expect(shaped.length).toBeGreaterThan(550);
    expect(
      shaped.filter(({ title }) => rootOf(title) === "Components").length,
    ).toBeGreaterThan(50);
    expect(
      shaped.filter(({ title }) => rootOf(title) === "Foundations").length,
    ).toBeGreaterThanOrEqual(topics.length);
    expect(order).not.toBeNull();
    expect(introduced.length).toBeGreaterThan(4);
  });

  it("fills every root the catalog documents", () => {
    expect(emptyRootFindings(filed, roots)).toEqual([]);
  });

  it("introduces the roots the catalog documents, in its order", () => {
    expect(introduced).toEqual(roots);
  });

  it("files every component under a declared category", () => {
    expect(shelfFindings(filed, "Components", categories, [3, 4])).toEqual([]);
  });

  it("files every foundation under a declared topic", () => {
    expect(shelfFindings(filed, "Foundations", topics, [2, 3])).toEqual([]);
  });

  it("writes every segment in Sentence case", () => {
    expect(caseFindings(shaped)).toEqual([]);
  });

  it("gives every story file a title of its own", () => {
    expect(sharedTitleFindings(shaped)).toEqual([]);
  });

  it("never makes one title both a leaf and a group", () => {
    expect(leafAndGroupFindings(shaped)).toEqual([]);
  });

  it("sorts the sidebar in the catalog's order", () => {
    expect(order?.roots).toEqual(roots);
    expect(order?.children.get("Components")).toEqual(categories);
    expect(order?.children.get("Foundations")).toEqual(topics);
  });
});

// An absence arm passes the same over a clean tree and a dead detector, so
// these plant each defect and read the detector directly.
describe("the sidebar detectors report what they are for", () => {
  const probe = join(frontendRoot, "src", "design-system", "probe.tsx");
  const script = join(frontendRoot, "src", "design-system", "probe.ts");

  it("sees a documented root with no story, or no docs page", () => {
    expect(
      emptyRootFindings(
        [
          { path: "a.mdx", title: "Get started/Introduction" },
          { path: "b.stories.tsx", title: "Shell/Top bar" },
        ],
        ["Get started", "Shell", "Records"],
      ),
    ).toEqual(["Records/ is documented and holds no story"]);
    expect(
      emptyRootFindings(
        [
          { path: "a.stories.tsx", title: "Get started/Introduction" },
          { path: "b.mdx", title: "Shell/Overview" },
        ],
        ["Get started", "Shell"],
      ),
    ).toEqual([
      "Get started/ is documented and holds no docs page",
      "Shell/ is documented and holds no story",
    ]);
  });

  it("reads the introduction's roots off its list, and nothing else", () => {
    const mdx = [
      "The **Theme** control is above.",
      "## Where things are",
      "",
      "- **Get started**: this page.",
      "- **Onboarding** and **Signed out**: the first run.",
      "  a wrapped line with **Not a root**",
      "",
      "## Checking a story",
      "- **Phone** at 390px.",
    ].join("\n");
    expect(introductionRoots(mdx)).toEqual([
      "Get started",
      "Onboarding",
      "Signed out",
    ]);
    expect(introductionRoots("- **Shell**: the frame.")).toEqual([]);
  });

  it("sees a title off the declared shelves, and a shelf with no story", () => {
    const shelves = ["Labels", "Messaging", "Loading"];
    expect(
      shelfFindings(
        [
          { path: "a", title: "Components/Labels/Tag pill" },
          { path: "b", title: "Components/Messaging/Toast" },
          { path: "c", title: "Components/Badges/Badge" },
          { path: "d", title: "Components/Labels" },
          { path: "e", title: "Patterns/Anything" },
          { path: "f", title: "Components/Labels/Tag pill/Row" },
          { path: "g", title: "Components/Labels/Tag pill/Row/Cell" },
        ],
        "Components",
        shelves,
        [3, 4],
      ),
    ).toEqual([
      'c files "Components/Badges/Badge" off the Components/ shelves the README declares',
      'd files "Components/Labels" off the Components/ shelves the README declares',
      'g files "Components/Labels/Tag pill/Row/Cell" off the Components/ shelves the README declares',
      "Components/Loading is declared and holds no story",
    ]);
  });

  it("sees a segment that is not Sentence case", () => {
    for (const segment of [
      "ListTable",
      "Stat Card",
      "stat card",
      "Stat  card",
      " Stat card",
      "",
      "ALERT",
      "Stat CARD",
      "1ALERT",
      "360View",
      "SMTP relay",
      "LDAP server",
      "Stat Äpfel",
      "ärger",
      "Ai pending",
      "Open in margince",
      "Ai-drafted reply",
      "Sign-In page",
      "Linkedin import",
      "Oauth return",
      "VCard import",
      "Vat mark",
    ]) {
      expect(isSentenceCase(segment)).toBe(false);
    }
    for (const segment of [
      "Stat card",
      "AI pending",
      "Company 360",
      "Open in Margince",
      "Sign-in page",
      "Ärger",
      "AI-drafted reply",
      "Margince’s core",
      "Margince's core",
      "IMAP connect form",
      "LinkedIn import",
      "OAuth return",
      "vCard import",
      "Voice DNA",
      "Add to Shortlist",
      "VAT mark",
    ]) {
      expect(isSentenceCase(segment)).toBe(true);
    }
    expect(
      caseFindings([
        { path: "a", title: "Components/Text and Data/List table" },
      ]),
    ).toEqual(['a: "Text and Data" is not Sentence case']);
  });

  it("holds a declared phrase to its spelling, whole or inside a segment", () => {
    for (const segment of ["MCP Apps", "Open in MCP Apps", "MCP Apps bridge"]) {
      expect(isSentenceCase(segment)).toBe(true);
    }
    for (const segment of [
      "Mcp Apps",
      "MCP apps x",
      "Open in MCP apps",
      "MCP Apps Bridge",
    ]) {
      expect(isSentenceCase(segment)).toBe(false);
    }
  });

  it("sees two files claiming one title, as Storybook's id folds it", () => {
    expect(
      sharedTitleFindings([
        { path: "a", title: "Components/Labels/Tag pill" },
        { path: "b", title: "Components/Labels/Row tags" },
        { path: "c", title: "Components/Labels/Tag/Pill" },
        { path: "d", title: "Components/Labels/AI tag" },
        { path: "e", title: "Components/Labels/Ai tag" },
      ]),
    ).toEqual([
      'id "components-labels-tag-pill" is claimed by a ("Components/Labels/Tag pill"), c ("Components/Labels/Tag/Pill")',
      'id "components-labels-ai-tag" is claimed by d ("Components/Labels/AI tag"), e ("Components/Labels/Ai tag")',
    ]);
  });

  it("sees a title that is both a leaf and a group", () => {
    expect(
      leafAndGroupFindings([
        { path: "a", title: "Components/Labels/Tag" },
        { path: "b", title: "Components/Labels/Tag/Row" },
        { path: "c", title: "Components/Labels/Tag pill" },
        { path: "d", title: "Components/Labels/AI mark" },
        { path: "e", title: "Components/Labels/Ai mark/Row" },
        { path: "f", title: "Components/Labels/Chip/Pill" },
        { path: "g", title: "Components/Labels/Chip pill/Row" },
      ]),
    ).toEqual([
      'a: "Components/Labels/Tag" is a leaf and also the group of "Components/Labels/Tag/Row"',
      'd: "Components/Labels/AI mark" is a leaf and also the group of "Components/Labels/Ai mark/Row"',
      'f: "Components/Labels/Chip/Pill" is a leaf and also the group of "Components/Labels/Chip pill/Row"',
    ]);
  });

  it("reads the sidebar order off the default-exported preview", () => {
    const preview = [
      'const fixture = { storySort: { order: ["Wrong"] } };',
      "const preview: Preview = { parameters: { options: { storySort: {",
      '  method: "alphabetical",',
      '  order: ["Foundations", ["Color"], "Components", ["Labels", "Messaging"], "Shell"],',
      "} } } };",
      "export default preview;",
    ].join("\n");
    const read = sidebarOrder(probe, preview);
    expect(read?.roots).toEqual(["Foundations", "Components", "Shell"]);
    expect(read?.children.get("Components")).toEqual(["Labels", "Messaging"]);
    expect(read?.children.get("Foundations")).toEqual(["Color"]);
  });

  it("reports no sidebar order rather than guessing one", () => {
    const preview = (order: string) =>
      `export default { parameters: { options: { storySort: { order: ${order} } } } };`;
    for (const order of [
      "ROOTS",
      "[...ROOTS]",
      '[["Color"], "Foundations"]',
      '["Components", [LABELS]]',
      '["Components", ["Labels"], ["Messaging"]]',
    ]) {
      expect(sidebarOrder(probe, preview(order))).toBe(null);
    }
    expect(
      sidebarOrder(probe, 'const p = { storySort: { order: ["Shell"] } };'),
    ).toBe(null);
    expect(sidebarOrder(probe, preview('["Shell"]'))).toEqual({
      roots: ["Shell"],
      children: new Map(),
    });
    expect(
      sidebarOrder(
        script,
        'export default <P>{ parameters: { options: { storySort: { order: ["Shell"] } } } };',
      )?.roots,
    ).toEqual(["Shell"]);
  });
});
