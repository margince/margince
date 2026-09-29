// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Catalog gate: README.md is the index of this directory, and an index is only
// worth reading if it is complete.
//
// The rule every other gate here supports is stated in frontend/AGENTS.md: read
// the catalog before you build anything you can see. Nothing mechanical can
// tell that the component somebody just wrote already existed under a different
// name — that part is the author's. But the author greps the catalog first, and
// a component the catalog never names cannot be found by a grep, so the one
// thing that CAN be held mechanically is that the index has an entry for every
// primitive in the tree.
//
// It did not. `richtext.tsx` shipped a rich-text editor with its own stylesheet,
// story and test, mounted in screens/contactdrawers.tsx, and the string
// "RichText" appeared nowhere in the catalog's 77KB. That is precisely the
// component a second author rebuilds: the noun is obvious, the grep comes back
// empty, and the duplicate looks reasonable in review.
//
// ## Arms, one subject
//
// 1. Every component in this directory is NAMED in the catalog table.
// 2. A row claiming a story of its own has a story file to claim.
// 3. Every story in the tree carries a title this file can read, filed under a
//    root the catalog documents.
// 4. Every `Components/` and `Foundations/` story sits on a shelf the catalog
//    declares, and every declared shelf holds one.
// 5. Titles under those roots are Sentence case, one per file, and never both a
//    leaf and a group.
// 6. `.storybook/preview.tsx` sorts the sidebar in the catalog's order.
//
// The third is here rather than beside the stories because the roots are
// declared in this file's subject. A story's title is the only thing that files
// it, and fe-uat keys on importPath, never on the title, so without this arm the
// shelf a reader looks on can stop being the shelf the story is on and nothing
// fails.
//
// ## What this gate deliberately does NOT decide
//
// It asks whether the NAME appears in the table region, not whether the row
// beside it is any good. A row's prose can be stale and this passes. That bar
// is chosen rather than settled for: the failure being held is a component
// nobody can find, and a name is exactly what a reader greps for. Judging the
// prose would need a second copy of the prose to judge it against, which is the
// defect this directory exists to avoid.
//
// The name may appear in any cell, the `For` column included — `FilePreview`
// documents its provider and hook inside its own row, and splitting a variant
// onto a line of its own would make the table longer without making it findable.
//
// ## Why this is a parser and not a shell gate
//
// The same reason native-controls.test.ts is: `export function Card` inside a
// comment is not an export, and a component is a PascalCase export that returns
// markup — both are properties of the language, and TypeScript's parser already
// knows them. A grep would have to be told, in awk, and would then have a
// second, worse answer to a question the compiler answers for free.

import { existsSync, readFileSync } from "node:fs";
import { basename, join, relative, resolve } from "node:path";
import { sanitize } from "storybook/internal/csf";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesUnder, parseSource } from "../../scripts/lib/source-tree";
import { storyTitle } from "../../scripts/lib/story-title";

const frontendRoot = resolve(__dirname, "..", "..");
const srcDir = join(frontendRoot, "src");
const dsDir = join(srcDir, "design-system");
const catalogPath = join(dsDir, "README.md");
const previewPath = join(frontendRoot, ".storybook", "preview.tsx");
const mainPath = join(frontendRoot, ".storybook", "main.ts");

// The catalog's two sections, found by their HEADINGS rather than by line number.
// A line number would be a second copy of the file's shape, and every edit to
// the prose above a table would move it.
const CATALOG_HEADING = "## What this directory already gives you";
const ROOTS_HEADING = "## Seeing them";

// A module that is not a primitive: the gate's own subject is what this
// directory SHIPS, and a test or a story ships nothing.
const NOT_SHIPPED = /\.(test|stories)\.tsx$/;

// readCatalog returns the raw text of the section under `heading`, up to the
// next heading of the same level. Sections rather than the whole file: a name
// that appears only in the prose at the top is a mention, not an entry, and the
// distinction is the whole point of having an index.
function catalogSection(heading: string): string {
  const lines = readFileSync(catalogPath, "utf8").split("\n");
  const start = lines.indexOf(heading);
  if (start < 0) {
    throw new Error(
      `${relative(frontendRoot, catalogPath)} has no "${heading}" section — ` +
        "this gate reads it, so renaming it silently empties the gate",
    );
  }
  const rest = lines.slice(start + 1);
  const end = rest.findIndex((line) => line.startsWith("## "));
  return (end < 0 ? rest : rest.slice(0, end)).join("\n");
}

const ALIGNMENT_RULE = /^\|?(\s*:?-+:?\s*\|)*\s*:?-+:?\s*\|?\s*$/;
const BLANK_ROW = /^[\s|]*$/;

type Table = { header: string; rows: string[][] };

// Every table in `section`. A header is the row an alignment rule follows, so
// one section may hold many.
function tablesIn(section: string): Table[] {
  const lines = section.split("\n");
  const tables: Table[] = [];
  let current: Table | undefined;
  for (const [index, line] of lines.entries()) {
    if (ALIGNMENT_RULE.test(line)) continue;
    if (!line.startsWith("|")) {
      current = undefined;
      continue;
    }
    if (BLANK_ROW.test(line)) {
      throw new Error(`a blank table row follows "${lines[index - 1]}"`);
    }
    const cells = line.slice(1).replace(/\|$/, "").split("|");
    if (ALIGNMENT_RULE.test(lines[index + 1] ?? "")) {
      current = { header: cells[0].trim(), rows: [] };
      tables.push(current);
    } else {
      current?.rows.push(cells);
    }
  }
  return tables;
}

function tableRows(section: string, header: string): string[][] {
  return tablesIn(section)
    .filter((table) => table.header === header)
    .flatMap((table) => table.rows);
}

// componentsIn returns every PascalCase export in `text` whose declaration
// contains markup.
//
// Both halves are load-bearing and neither is sufficient alone. PascalCase
// without JSX admits `MAX_DPR`'s neighbours and every exported type; JSX
// without PascalCase admits the local render helpers a big module keeps to
// itself, which are not primitives and have nothing to document.
//
// A module publishes a component two ways, and a gate that knows only one goes
// blind to the other while still reporting PASS — the one direction a census
// must not be wrong in. `export function Card` carries the keyword on the
// declaration; `export { Card }` carries it in a list elsewhere in the file,
// and `export { Card as Plate }` publishes a name the declaration never spells.
// The PUBLIC name is the one the catalog has to carry, because it is what a
// caller imports and therefore what a reader greps for.
function componentsIn(path: string, text: string): string[] {
  const source = parseSource(path, text);
  const declarations = new Map<string, ts.Node>();
  const published: [string, string][] = [];
  for (const statement of source.statements) {
    for (const [local, declaration] of declaredValues(statement)) {
      declarations.set(local, declaration);
      if (isExported(statement)) published.push([local, local]);
    }
    // A re-export names a module of its own (`export { x } from "./y"`), and
    // what it publishes is that module's, not this one's — judging it here
    // would report a neighbour's component against this file.
    if (
      ts.isExportDeclaration(statement) &&
      statement.moduleSpecifier === undefined &&
      statement.exportClause !== undefined &&
      ts.isNamedExports(statement.exportClause)
    ) {
      for (const element of statement.exportClause.elements) {
        published.push([
          (element.propertyName ?? element.name).text,
          element.name.text,
        ]);
      }
    }
  }
  return published.flatMap(([local, name]) => {
    const declaration = declarations.get(local);
    return declaration !== undefined &&
      /^[A-Z][A-Za-z0-9]*$/.test(name) &&
      rendersMarkup(declaration)
      ? [name]
      : [];
  });
}

// The `export` keyword on the statement itself, read off its modifiers.
// `getCombinedModifierFlags` wants a Declaration and a Statement is not one, so
// reaching it would mean asserting something the parser has not said.
function isExported(node: ts.Statement): boolean {
  return (
    ts.canHaveModifiers(node) &&
    (ts
      .getModifiers(node)
      ?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword) ??
      false)
  );
}

// declaredValues yields the name and declaration node of every VALUE a
// statement declares. A type alias and an interface declare no value, so a
// `type Card = …` cannot stand in for the component the catalog is missing.
function declaredValues(node: ts.Statement): [string, ts.Node][] {
  if (ts.isFunctionDeclaration(node)) {
    return node.name ? [[node.name.text, node]] : [];
  }
  if (ts.isVariableStatement(node)) {
    return node.declarationList.declarations.flatMap((declaration) =>
      ts.isIdentifier(declaration.name)
        ? [[declaration.name.text, declaration] as [string, ts.Node]]
        : [],
    );
  }
  return [];
}

function rendersMarkup(node: ts.Node): boolean {
  let markup = false;
  const visit = (child: ts.Node): void => {
    if (
      ts.isJsxElement(child) ||
      ts.isJsxSelfClosingElement(child) ||
      ts.isJsxFragment(child)
    ) {
      markup = true;
    }
    if (!markup) ts.forEachChild(child, visit);
  };
  visit(node);
  return markup;
}

// The roots the shape rules below hold.
const SHAPED_ROOTS = ["Foundations", "Components"];

// Words that keep their declared spelling wherever they stand in a segment.
const PROPER_NOUNS = new Map([["Margince", "the product's name"]]);
const ACRONYMS = new Map([
  ["AI", "the agent tier, as the product's copy writes it"],
]);

type Filed = { path: string; title: string };

function rootOf(title: string): string {
  return title.split("/")[0];
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

function isSentenceCase(segment: string): boolean {
  if (!/^\S+( \S+)*$/.test(segment)) return false;
  const parts = segment
    .split(" ")
    .flatMap((word) => word.split("-"))
    .map((part) => part.replace(/['’]s$/u, ""));
  return parts.every((part, index) => {
    const declared = DECLARED.find(
      (spelling) => spelling.toLowerCase() === part.toLowerCase(),
    );
    if (declared !== undefined) return part === declared;
    if (/^\p{N}+$/u.test(part)) return true;
    return index === 0
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

function unwrapped(expression: ts.Expression): ts.Expression {
  let node = expression;
  while (
    ts.isParenthesizedExpression(node) ||
    ts.isAsExpression(node) ||
    ts.isSatisfiesExpression(node) ||
    ts.isTypeAssertionExpression(node)
  ) {
    node = node.expression;
  }
  return node;
}

function defaultExportObject(
  source: ts.SourceFile,
): ts.ObjectLiteralExpression | undefined {
  const exported = source.statements.find(ts.isExportAssignment);
  if (exported === undefined) return undefined;
  const value = unwrapped(exported.expression);
  const resolved = ts.isIdentifier(value)
    ? source.statements
        .filter(ts.isVariableStatement)
        .flatMap((statement) => statement.declarationList.declarations)
        .find(
          (declaration) =>
            ts.isIdentifier(declaration.name) &&
            declaration.name.text === value.text,
        )?.initializer
    : value;
  const object = resolved && unwrapped(resolved);
  return object && ts.isObjectLiteralExpression(object) ? object : undefined;
}

function literalAt(
  object: ts.ObjectLiteralExpression | undefined,
  path: string[],
): ts.Expression | undefined {
  let value: ts.Expression | undefined = object;
  for (const key of path) {
    if (value === undefined || !ts.isObjectLiteralExpression(value)) {
      return undefined;
    }
    const property = value.properties
      .filter(ts.isPropertyAssignment)
      .find(
        ({ name }) =>
          (ts.isIdentifier(name) || ts.isStringLiteral(name)) &&
          name.text === key,
      );
    value = property && unwrapped(property.initializer);
  }
  return value;
}

// Null for a glob it cannot turn into suffixes, so the census fails closed.
function storySuffixes(path: string, text: string): string[] | null {
  const stories = literalAt(defaultExportObject(parseSource(path, text)), [
    "stories",
  ]);
  if (stories === undefined || !ts.isArrayLiteralExpression(stories)) {
    return null;
  }
  const suffixes: string[] = [];
  for (const element of stories.elements) {
    if (!ts.isStringLiteralLike(element)) return null;
    const glob = /^\.\.\/src\/\*\*\/\*(\.[\w.]+)\.@\(([\w|]+)\)$/.exec(
      element.text,
    );
    if (glob === null) return null;
    suffixes.push(...glob[2].split("|").map((ext) => `${glob[1]}.${ext}`));
  }
  return suffixes.length > 0 ? suffixes : null;
}

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
  const roots: string[] = [];
  const children = new Map<string, string[]>();
  for (const element of order.elements) {
    if (ts.isStringLiteralLike(element)) {
      roots.push(element.text);
      continue;
    }
    const root = roots.at(-1);
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
  return roots.length > 0 ? { roots, children } : null;
}

const catalogTable = catalogSection(CATALOG_HEADING);
const rootsSection = catalogSection(ROOTS_HEADING);

const rootRows = tableRows(rootsSection, "Root");

// One row carries two roots, so every backticked `Name/` in the first cell
// counts rather than the cell itself.
const documentedRootOrder = rootRows.flatMap((cells) =>
  [...cells[0].matchAll(/`([^`/]+)\/`/g)].map((match) => match[1]),
);
const documentedRoots = new Set(documentedRootOrder);

const categories = tableRows(rootsSection, "Category").map((cells) =>
  cells[0].trim(),
);

// The Foundations row names its topics, and nothing else, in backticks.
const topics = [
  ...(
    rootRows.find((cells) => cells[0].includes("`Foundations/`"))?.[1] ?? ""
  ).matchAll(/`([^`]+)`/g),
].map((match) => match[1]);

// Catalog groups that are not a `Components/` category.
const UNSHELVED_GROUPS = ["Foundations", "Libraries"];

function catalogGroups(section: string): string[] {
  return section
    .split("\n")
    .filter((line) => line.startsWith("### "))
    .map((line) => line.slice(4).trim());
}

const primitiveModules = filesUnder(dsDir).filter(
  (path) => path.endsWith(".tsx") && !NOT_SHIPPED.test(basename(path)),
);

const storySuffixList = storySuffixes(mainPath, readFileSync(mainPath, "utf8"));

const storyFiles = filesUnder(srcDir).filter((path) =>
  (storySuffixList ?? []).some((suffix) => path.endsWith(suffix)),
);

describe("the catalog indexes this directory", () => {
  // A census that reads a smaller tree reports the same word a clean one does.
  // Both floors are well under today's counts and exist to fail when the walk
  // breaks, not to pin a number somebody must maintain.
  it("reads the tree it is pointed at", () => {
    expect(primitiveModules.length).toBeGreaterThan(30);
    expect(storySuffixList).not.toBeNull();
    expect(storyFiles.length).toBeGreaterThan(100);
    expect(documentedRoots.size).toBeGreaterThan(4);
    expect(catalogTable.length).toBeGreaterThan(10_000);
    expect(tableRows(catalogTable, "Primitive").length).toBeGreaterThan(100);
  });

  it("reads every table in its sections, and no other kind", () => {
    const unread = [
      ...tablesIn(catalogTable)
        .filter(({ header }) => header !== "Primitive")
        .map(({ header }) => `${CATALOG_HEADING}: a "${header}" table`),
      ...tablesIn(rootsSection)
        .filter(({ header }) => header !== "Root" && header !== "Category")
        .map(({ header }) => `${ROOTS_HEADING}: a "${header}" table`),
    ];
    expect(unread).toEqual([]);
  });

  it("names every component this directory ships", () => {
    const unnamed = primitiveModules.flatMap((path) => {
      const text = readFileSync(path, "utf8");
      return componentsIn(path, text)
        .filter((name) => !new RegExp(`\\b${name}\\b`).test(catalogTable))
        .map((name) => `${relative(frontendRoot, path)} exports ${name}`);
    });
    expect(unnamed).toEqual([]);
  });

  // A BARE ✅ is the claim being held: the table's own convention is that a
  // qualifier names where the coverage is instead — `✅ (`Value inputs`)` for a
  // primitive exercised by a neighbour's story, `via `IconAction`` for one with
  // no story of its own. Reading those qualifiers would mean parsing the prose,
  // and a gate that keeps its own copy of the prose is the defect this file is
  // about. So the arm holds the one claim that is unambiguous, and the
  // qualified rows are the author's word.
  it("claims a story of its own only where one exists", () => {
    const lying = tableRows(catalogTable, "Primitive").flatMap((cells) => {
      const [primitive, , file, story] = cells;
      if (story?.trim() !== "✅") return [];
      const module = file.trim().replace(/`/g, "");
      if (!module.endsWith(".tsx") || module.includes("/")) return [];
      const stories = join(dsDir, module.replace(/\.tsx$/, ".stories.tsx"));
      return existsSync(stories)
        ? []
        : [
            `${primitive.trim()} claims ✅ but ${basename(stories)} does not exist`,
          ];
    });
    expect(lying).toEqual([]);
  });

  it("reads a title off every story file", () => {
    const untitled = storyFiles
      .filter((path) => storyTitle(path, readFileSync(path, "utf8")) === null)
      .map((path) => relative(frontendRoot, path));
    expect(untitled).toEqual([]);
  });

  it("files every story under a documented root", () => {
    const stray = storyFiles.flatMap((path) => {
      const title = storyTitle(path, readFileSync(path, "utf8"));
      if (title === null) return [];
      const root = title.split("/")[0];
      return documentedRoots.has(root)
        ? []
        : [`${relative(frontendRoot, path)} files under ${root}/`];
    });
    expect(stray).toEqual([]);
  });
});

describe("the sidebar is shelved the way the catalog says", () => {
  const filed = storyFiles.flatMap((path) => {
    const title = storyTitle(path, readFileSync(path, "utf8"));
    return title === null
      ? []
      : [{ path: relative(frontendRoot, path), title }];
  });
  const shaped = filed.filter(({ title }) =>
    SHAPED_ROOTS.includes(rootOf(title)),
  );
  const order = sidebarOrder(previewPath, readFileSync(previewPath, "utf8"));

  it("reads the shelves and titles it holds", () => {
    expect(categories.length).toBeGreaterThan(5);
    expect(topics.length).toBeGreaterThan(1);
    expect(
      shaped.filter(({ title }) => rootOf(title) === "Components").length,
    ).toBeGreaterThan(50);
    expect(
      shaped.filter(({ title }) => rootOf(title) === "Foundations").length,
    ).toBeGreaterThanOrEqual(topics.length);
    expect(order).not.toBeNull();
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

  it("groups the catalog by the same categories, in order", () => {
    const groups = catalogGroups(catalogTable).filter(
      (group) => !UNSHELVED_GROUPS.includes(group),
    );
    expect(groups).toEqual(categories);
  });

  it("sorts the sidebar in the catalog's order", () => {
    expect(order?.roots).toEqual(documentedRootOrder);
    expect(order?.children.get("Components")).toEqual(categories);
    expect(order?.children.get("Foundations")).toEqual(topics);
  });
});

// A gate asserting a shape is ABSENT passes identically over a clean tree and
// over a detector that has stopped detecting. These plant each defect and read
// the detector directly.
describe("the detectors report what they are for", () => {
  const probe = join(dsDir, "probe.tsx");
  const script = join(dsDir, "probe.ts");

  it("sees an exported component", () => {
    expect(
      componentsIn(probe, "export function Card() {\n  return <div />;\n}"),
    ).toEqual(["Card"]);
    expect(componentsIn(probe, "export const Card = () => <div />;")).toEqual([
      "Card",
    ]);
  });

  it("sees a component published through an export list", () => {
    expect(
      componentsIn(probe, "const Card = () => <div />;\nexport { Card };"),
    ).toEqual(["Card"]);
  });

  it("sees an export list's PUBLIC name, not the local one", () => {
    // `export { Card as Plate }` is imported as Plate, so Plate is the noun a
    // reader greps the catalog for. Reporting Card would send them looking for
    // a name no caller ever writes.
    expect(
      componentsIn(
        probe,
        "const Card = () => <div />;\nexport { Card as Plate };",
      ),
    ).toEqual(["Plate"]);
  });

  it("does not see a re-export from another module", () => {
    // What `export { Card } from "./card"` publishes belongs to that module.
    // Judging it here would report a neighbour's component against this file.
    expect(componentsIn(probe, 'export { Card } from "./card";')).toEqual([]);
  });

  it("does not see a component in a comment", () => {
    expect(
      componentsIn(probe, "// export function Card() { return <div />; }"),
    ).toEqual([]);
  });

  it("does not see a helper that is not exported", () => {
    expect(
      componentsIn(probe, "function Card() {\n  return <div />;\n}"),
    ).toEqual([]);
  });

  it("does not see an export that renders nothing", () => {
    expect(componentsIn(probe, "export const MAX_DPR = 2;")).toEqual([]);
    expect(componentsIn(probe, "export type Card = { id: string };")).toEqual(
      [],
    );
  });

  it("does not see a lowercase render helper", () => {
    expect(
      componentsIn(probe, "export function row() {\n  return <tr />;\n}"),
    ).toEqual([]);
  });

  it("reads the title off the default export, not the first match", () => {
    const source = [
      'const fixture = { title: "Commercial terms v4" };',
      'const meta = { title: "Records/Deal room/Documents and threads" };',
      "export default meta;",
    ].join("\n");
    expect(storyTitle(probe, source)).toBe(
      "Records/Deal room/Documents and threads",
    );
  });

  it("reads a title off an inline default export", () => {
    expect(
      storyTitle(probe, 'export default { title: "Shell/Top bar" };'),
    ).toBe("Shell/Top bar");
  });

  it("reads a title through the type-only wrappers", () => {
    // Each of these changes nothing about the object underneath. A scanner that
    // stopped at one would read no title, and an absent title is skipped rather
    // than reported — so this form would walk past the root check.
    for (const meta of [
      'const meta = { title: "Shell/Top bar" } satisfies Meta<typeof Bar>;',
      'const meta = { title: "Shell/Top bar" } as Meta<typeof Bar>;',
      'const meta = ({ title: "Shell/Top bar" });',
    ]) {
      expect(storyTitle(probe, `${meta}\nexport default meta;`)).toBe(
        "Shell/Top bar",
      );
    }
    expect(
      storyTitle(
        probe,
        'export default { title: "Shell/Top bar" } satisfies Meta<typeof Bar>;',
      ),
    ).toBe("Shell/Top bar");
  });

  it("reads a title whichever way the key and value are written", () => {
    for (const meta of [
      'const meta = { "title": "Shell/Top bar" };',
      'const meta = { title: "Shell/Top bar" as const };',
      'const meta = { title: ("Shell/Top bar") };',
      'const meta = { "title": "Shell/Top bar" as const };',
    ]) {
      expect(storyTitle(probe, `${meta}\nexport default meta;`)).toBe(
        "Shell/Top bar",
      );
    }
  });

  it("reports no title rather than resolving a computed key", () => {
    // What a computed key evaluates to is not a question the parser can answer,
    // and a guess would be worse than the honest null.
    expect(
      storyTitle(
        probe,
        'const k = "title";\nconst meta = { [k]: "Shell/Top bar" };\nexport default meta;',
      ),
    ).toBe(null);
  });

  it("reports no title rather than guessing when there is no default export", () => {
    expect(storyTitle(probe, 'const meta = { title: "Shell/Top bar" };')).toBe(
      null,
    );
  });

  it("reports no title for a meta a literal read cannot resolve", () => {
    for (const meta of [
      `const meta = { title: \`Shell/\${bar}\` };\nexport default meta;`,
      'const meta = { title: "Shell/" + bar };\nexport default meta;',
      "const meta = { title: TITLE };\nexport default meta;",
      "const meta = { title } as Meta;\nexport default meta;",
      "const meta = {};\nexport default meta;",
      'const meta = { title: "Shell/Top bar" };\nexport { meta as default };',
    ]) {
      expect(storyTitle(probe, meta)).toBe(null);
    }
  });

  it("reads every table under its header, whatever its kind", () => {
    const section = [
      "| Root | What |",
      "|---|---|",
      "| `A/` | one |",
      "",
      "| Hook | What |",
      "|---|---",
      "| `useX` | a hook |",
      "",
      "| Root | What |",
      "| --- | --- |",
      "| `B/` | two |",
    ].join("\n");
    expect(tablesIn(section).map(({ header }) => header)).toEqual([
      "Root",
      "Hook",
      "Root",
    ]);
    expect(tableRows(section, "Root").map((cells) => cells[0].trim())).toEqual([
      "`A/`",
      "`B/`",
    ]);
    expect(tableRows(section, "Hook")).toEqual([[" `useX` ", " a hook "]]);
  });

  it("reads the catalog's group headings", () => {
    expect(
      catalogGroups(
        "### Foundations\n\n| a |\n### Labels\n#### Not one\n### Messaging",
      ),
    ).toEqual(["Foundations", "Labels", "Messaging"]);
  });

  it("refuses a blank row inside a table", () => {
    const section = [
      "| Category | What |",
      "|---|---|",
      "| Labels | pills |",
      "| | |",
      "| Messaging | notes |",
    ].join("\n");
    expect(() => tableRows(section, "Category")).toThrow(
      'a blank table row follows "| Labels | pills |"',
    );
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
      "MCP server",
      "Stat Äpfel",
      "ärger",
      "Ai pending",
      "Open in margince",
      "Ai-drafted reply",
      "Sign-In page",
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
    ]) {
      expect(isSentenceCase(segment)).toBe(true);
    }
    expect(
      caseFindings([
        { path: "a", title: "Components/Text and Data/List table" },
      ]),
    ).toEqual(['a: "Text and Data" is not Sentence case']);
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

  it("reads the story suffixes off the stories globs", () => {
    const main = (globs: string) =>
      `const config = { stories: [${globs}] } satisfies Config;\nexport default config;`;
    expect(
      storySuffixes(probe, main('"../src/**/*.stories.@(ts|tsx)"')),
    ).toEqual([".stories.ts", ".stories.tsx"]);
    expect(
      storySuffixes(
        script,
        'export default <C>{ stories: ["../src/**/*.stories.@(tsx)"] };',
      ),
    ).toEqual([".stories.tsx"]);
    for (const globs of [
      '"../src/**/*.mdx"',
      '"../src/**/*.stories.tsx"',
      "GLOBS",
      '"../src/**/*.stories.@(ts|tsx)", "../extensions/**/*.stories.tsx"',
    ]) {
      expect(storySuffixes(probe, main(globs))).toBe(null);
    }
    expect(
      storySuffixes(
        probe,
        'export const stories = ["../src/**/*.stories.@(tsx)"];',
      ),
    ).toBe(null);
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
    const order = sidebarOrder(probe, preview);
    expect(order?.roots).toEqual(["Foundations", "Components", "Shell"]);
    expect(order?.children.get("Components")).toEqual(["Labels", "Messaging"]);
    expect(order?.children.get("Foundations")).toEqual(["Color"]);
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
