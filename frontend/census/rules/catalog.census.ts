// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Catalog gate: README.md is the index of src/design-system/, and an index is
// worth reading only if it is complete.
//
// No tool can tell that a new component already exists under another name.
// The author greps the catalog first, and a component the catalog never names
// cannot be found by a grep. So the index needs an entry for every primitive.
//
// 1. Every component in the directory is named in the catalog table.
// 2. A row claiming a story of its own has a story file to claim.
// 3. Every story in the tree carries a readable title, filed under a root the
//    catalog documents. fe-uat keys on importPath, so nothing else holds this.
// 4. The catalog's groups are the `Components/` categories, in order.
//
// It asks whether a name appears in the table, not whether its row is good.
// Judging the prose would need a second copy of the prose.

import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import ts from "typescript";
import {
  CATALOG_HEADING,
  ROOTS_HEADING,
  readDesignCatalog,
  tableRows,
  tablesIn,
} from "../../scripts/lib/design-catalog";
import { parseSource } from "../../scripts/lib/source-tree";
import { storySuffixes } from "../../scripts/lib/story-files";
import { defaultExportObject, literalAt } from "../../scripts/lib/story-title";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  frontendRoot,
  srcDir,
} from "../tree";

const dsDir = join(srcDir, "design-system");

// A test or a story ships nothing, so it is no primitive.
const NOT_SHIPPED = /\.(test|stories)\.tsx$/;

// Catalog groups that are not a `Components/` category.
const UNSHELVED_GROUPS = ["Foundations", "Libraries"];

export function catalogGroups(section: string): string[] {
  return section
    .split("\n")
    .filter((line) => line.startsWith("### "))
    .map((line) => line.slice(4).trim());
}

// componentsIn returns every PascalCase export in `text` whose declaration
// contains markup. PascalCase alone admits every exported type, and markup
// alone admits a module's local render helpers.
//
// `export { Card as Plate }` publishes a name the declaration never spells.
// The public name is the one a caller imports, so the catalog carries it.
export function componentsIn(path: string, text: string): string[] {
  const source = parseSource(path, text);
  const declarations = new Map<string, ts.Node>();
  const published: [string, string][] = [];
  for (const statement of source.statements) {
    for (const [local, declaration] of declaredValues(statement)) {
      declarations.set(local, declaration);
      if (isExported(statement)) published.push([local, local]);
    }
    // What `export { x } from "./y"` publishes is that module's, not this one's.
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

// declaredValues yields the name and declaration node of every value a
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

// A bare ✅ is met by the module's co-located story. A story named for a
// component in the row's first cell also meets it, if it imports the module.
export function falseStoryClaims(
  rows: string[][],
  read: (story: string) => string | null,
): string[] {
  return rows.flatMap(([primitive, , file, story]) => {
    if (story?.trim() !== "✅") return [];
    const module = file.trim().replace(/`/g, "");
    if (!module.endsWith(".tsx") || module.includes("/")) return [];
    const stem = module.replace(/\.tsx$/, "");
    const named = [...primitive.matchAll(/`([A-Z][A-Za-z0-9]*)`/g)].map(
      ([, name]) => `${name.toLowerCase()}.stories.tsx`,
    );
    const met =
      read(`${stem}.stories.tsx`) !== null ||
      named.some((candidate) => importsModule(candidate, read, stem));
    return met
      ? []
      : [
          `${primitive.trim()} claims ✅ but no story imports ${module}: ${[...named, `${stem}.stories.tsx`].join(", ")}`,
        ];
  });
}

function importsModule(
  story: string,
  read: (story: string) => string | null,
  stem: string,
): boolean {
  const text = read(story);
  if (text === null) return false;
  return parseSource(join(dsDir, story), text).statements.some(
    (statement) =>
      ts.isImportDeclaration(statement) &&
      ts.isStringLiteral(statement.moduleSpecifier) &&
      statement.moduleSpecifier.text === `./${stem}` &&
      importsValue(statement.importClause),
  );
}

// A type-only import renders nothing, and neither does a bare side-effect one.
function importsValue(clause: ts.ImportClause | undefined): boolean {
  if (clause === undefined || clause.isTypeOnly) return false;
  if (clause.name !== undefined) return true;
  const bindings = clause.namedBindings;
  if (bindings === undefined) return false;
  return (
    ts.isNamespaceImport(bindings) ||
    bindings.elements.some((element) => !element.isTypeOnly)
  );
}

// ─── Story titles ──────────────────────────────────────────────────────────

/** The title a CSF story file claims at its default export, or null. */
export function csfTitle(source: ts.SourceFile): string | null {
  const title = literalAt(defaultExportObject(source), ["title"]);
  return title !== undefined && ts.isStringLiteralLike(title)
    ? title.text
    : null;
}

// Storybook's own MDX analyser files a docs page. It is async and a rule is
// not, so it runs in one child process over every page at once.
const MDX_TITLES = `
import { readFileSync } from "node:fs";
const { analyzeMdx } = await import("storybook/internal/core-server");
const titles = [];
for (const path of JSON.parse(process.argv[1])) {
  try {
    titles.push((await analyzeMdx(readFileSync(path, "utf8"))).title ?? null);
  } catch (refused) {
    if (!(refused instanceof Error)) throw refused;
    titles.push(null);
  }
}
process.stdout.write(JSON.stringify(titles));
`;

function mdxTitles(paths: readonly string[]): (string | null)[] {
  if (paths.length === 0) return [];
  const out = execFileSync(
    process.execPath,
    ["--input-type=module", "-e", MDX_TITLES, JSON.stringify(paths)],
    { cwd: frontendRoot, encoding: "utf8" },
  );
  const titles: unknown = JSON.parse(out);
  if (
    !Array.isArray(titles) ||
    !titles.every((title) => title === null || typeof title === "string")
  ) {
    throw new Error(
      "the MDX analyser answered with something other than titles",
    );
  }
  return titles;
}

const escaped = (suffix: string) =>
  suffix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

type TitledStory = { rel: string; title: string | null };

type Stories = { suffixes: string[] | null; stories: TitledStory[] };

// Floors and findings both read the stories; the docs pages cost a process.
const storiesByTree = new WeakMap<CensusTree, Stories>();

/** Every story file and docs page Storybook loads, read off its main.ts globs. */
export function storiesOf(tree: CensusTree): Stories {
  const known = storiesByTree.get(tree);
  if (known) return known;
  const found = readStories(tree);
  storiesByTree.set(tree, found);
  return found;
}

function readStories(tree: CensusTree): Stories {
  const main = join(frontendRoot, ".storybook", "main.ts");
  const suffixes = storySuffixes(main, tree.at(main)?.text ?? "");
  if (suffixes === null) return { suffixes, stories: [] };
  const matches = new RegExp(`(${suffixes.map(escaped).join("|")})$`);
  const loaded = tree.files.filter(
    (file) => file.area === "src" && matches.test(file.rel),
  );
  const csf = loaded.flatMap((file) =>
    file.source ? [{ rel: file.rel, title: csfTitle(file.source) }] : [],
  );
  const pages = loaded.filter((file) => file.rel.endsWith(".mdx"));
  const docs = mdxTitles(pages.map((file) => file.path)).map(
    (title, index) => ({ rel: pages[index].rel, title }),
  );
  return { suffixes, stories: [...csf, ...docs] };
}

// ─── The rule ──────────────────────────────────────────────────────────────

function catalogFindings(
  corpus: readonly CensusFile[],
  tree: CensusTree,
): string[] {
  const { catalogTable, rootsSection, roots, categories } =
    readDesignCatalog(frontendRoot);
  const unread = [
    ...tablesIn(catalogTable)
      .filter(({ header }) => header !== "Primitive")
      .map(
        ({ header }) => `${CATALOG_HEADING}: a "${header}" table is not read`,
      ),
    ...tablesIn(rootsSection)
      .filter(({ header }) => header !== "Root" && header !== "Category")
      .map(({ header }) => `${ROOTS_HEADING}: a "${header}" table is not read`),
  ];
  const unnamed = corpus.flatMap((file) =>
    componentsIn(file.path, file.text)
      .filter((name) => !new RegExp(`\\b${name}\\b`).test(catalogTable))
      .map(
        (name) => `${file.rel} exports ${name}, and the catalog never names it`,
      ),
  );
  const read = (story: string) => tree.at(join(dsDir, story))?.text ?? null;
  const groups = catalogGroups(catalogTable).filter(
    (group) => !UNSHELVED_GROUPS.includes(group),
  );
  return [
    ...unread,
    ...unnamed,
    ...falseStoryClaims(tableRows(catalogTable, "Primitive"), read),
    ...(groups.join("|") === categories.join("|")
      ? []
      : [
          `the catalog groups are ${groups.join(", ")}; the categories are ${categories.join(", ")}`,
        ]),
    ...storyFindings(tree, new Set(roots)),
  ];
}

function storyFindings(tree: CensusTree, roots: ReadonlySet<string>): string[] {
  return storiesOf(tree).stories.flatMap(({ rel, title }) => {
    if (title === null) return [`${rel} carries no title this gate can read`];
    const root = title.split("/")[0];
    return roots.has(root) ? [] : [`${rel} files under ${root}/`];
  });
}

export const rule: CensusRule = {
  name: "catalog",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        dirname(file.path).startsWith(dsDir) &&
        file.rel.endsWith(".tsx") &&
        !NOT_SHIPPED.test(file.rel),
    ),
  // Well under today's counts: each fails when the walk breaks.
  floors: (corpus, tree) => {
    const { catalogTable, roots } = readDesignCatalog(frontendRoot);
    const { suffixes, stories } = storiesOf(tree);
    return [
      ...(corpus.length > 30
        ? []
        : ["fewer than 30 primitive modules were read"]),
      ...(suffixes !== null
        ? []
        : [".storybook/main.ts names a story glob this gate cannot read"]),
      ...(stories.length > 100 ? [] : ["fewer than 100 story files were read"]),
      ...(roots.length > 4
        ? []
        : ["the catalog documents four roots or fewer"]),
      ...(catalogTable.length > 10_000
        ? []
        : ["the catalog table is under 10 000 characters"]),
      ...(tableRows(catalogTable, "Primitive").length > 100
        ? []
        : ["the catalog table has 100 rows or fewer"]),
    ];
  },
  across: catalogFindings,
  advice:
    "src/design-system/README.md is the catalog. Name every shipped component " +
    "in its table, claim a bare ✅ only where a story imports the row's module, " +
    "give every story a title under a documented root, and keep the groups in " +
    "the order of the Category table.",
};
