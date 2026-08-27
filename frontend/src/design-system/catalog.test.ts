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
// story and test, mounted in screens/persondrawers.tsx, and the string
// "RichText" appeared nowhere in the catalog's 77KB. That is precisely the
// component a second author rebuilds: the noun is obvious, the grep comes back
// empty, and the duplicate looks reasonable in review.
//
// ## Three arms, one subject
//
// 1. Every component in this directory is NAMED in the catalog table.
// 2. A row claiming a story of its own has a story file to claim.
// 3. Every story in the tree files under a root the catalog documents.
//
// The third is here rather than beside the stories because the roots are
// declared in this file's subject: the catalog names eight, and the sidebar had
// grown to fourteen — a retired `Screens/` root still carrying nineteen files,
// four roots holding one story each, and a `Design system/` separated from
// `Design System/` by the case of one letter. A story's title is the only thing
// that files it, fe-uat keys on importPath and never on the title, and so
// nothing failed while the shelf a reader looks on stopped being the shelf the
// story is on.
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
// The name may appear in any cell, the `For` column included — `BriefItemCard`
// documents `BriefItemCardPending` inside its own row, and splitting a variant
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
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesUnder, scriptKindFor } from "../../scripts/lib/source-tree";

const frontendRoot = resolve(__dirname, "..", "..");
const srcDir = join(frontendRoot, "src");
const dsDir = join(srcDir, "design-system");
const catalogPath = join(dsDir, "README.md");

// The catalog's two tables, found by their HEADINGS rather than by line number.
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

// tableRows returns the markdown table rows of a section, header and alignment
// rule dropped, each split into its cells.
function tableRows(section: string): string[][] {
  return section
    .split("\n")
    .filter((line) => line.startsWith("|"))
    .map((line) => line.slice(1).replace(/\|$/, "").split("|"))
    .filter((cells) => !/^[\s|:-]*$/.test(cells.join("|")))
    .slice(1);
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
  const source = ts.createSourceFile(
    path,
    text,
    ts.ScriptTarget.Latest,
    true,
    scriptKindFor(path),
  );
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

// storyTitle returns the sidebar path a story file claims, or null.
//
// It resolves the DEFAULT EXPORT rather than reading the first `title:` in the
// file, because a story's fixture data carries titles of its own —
// dealroomthreads.stories.tsx opens with `title: "Commercial terms v4"`, the
// name of a document in the fixture, and a scanner reading the first match
// would file that story under a root called "Commercial terms v4".
function storyTitle(path: string, text: string): string | null {
  const source = ts.createSourceFile(
    path,
    text,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const exported = source.statements.find(ts.isExportAssignment);
  if (!exported) return null;
  const named = unwrap(exported.expression);
  const meta = ts.isIdentifier(named)
    ? metaObjectNamed(source, named.text)
    : named;
  if (!meta || !ts.isObjectLiteralExpression(meta)) return null;
  for (const property of meta.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    if (propertyKey(property.name) !== "title") continue;
    const value = unwrap(property.initializer);
    if (ts.isStringLiteralLike(value)) return value.text;
  }
  return null;
}

// The key, whichever way it is written. `{ title: … }` and `{ "title": … }` are
// the same property, but reading the name's SOURCE TEXT compares the quotes
// too, so the quoted spelling matched nothing and the story fell out of the
// root check — skipped rather than reported, the one direction this gate must
// not be wrong in. A computed key is deliberately not resolved: what it
// evaluates to is not a question the parser can answer, and guessing would be
// worse than the honest null.
function propertyKey(name: ts.PropertyName): string | null {
  if (ts.isIdentifier(name) || ts.isStringLiteralLike(name)) return name.text;
  return null;
}

function metaObjectNamed(
  source: ts.SourceFile,
  name: string,
): ts.Expression | undefined {
  for (const statement of source.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (ts.isIdentifier(declaration.name) && declaration.name.text === name) {
        return declaration.initializer && unwrap(declaration.initializer);
      }
    }
  }
  return undefined;
}

// The type-only wrappers a story's metadata may be written through. They change
// nothing about the object underneath, so a scanner that stops at them reads no
// title — and an absent title is SKIPPED by the root check rather than
// reported, which is the one direction this gate must not be wrong in. This
// tree already writes `as const satisfies` elsewhere, so the form is one edit
// away from appearing here.
function unwrap(expression: ts.Expression): ts.Expression {
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

const catalogTable = catalogSection(CATALOG_HEADING);
const rootsSection = catalogSection(ROOTS_HEADING);

// The roots the catalog declares, read out of its own table. One row carries
// three of them, which is why every backticked `Name/` in the first cell counts
// rather than the cell itself.
const documentedRoots = new Set(
  tableRows(rootsSection).flatMap((cells) =>
    [...cells[0].matchAll(/`([^`/]+)\/`/g)].map((match) => match[1]),
  ),
);

const primitiveModules = filesUnder(dsDir).filter(
  (path) => path.endsWith(".tsx") && !NOT_SHIPPED.test(basename(path)),
);

const storyFiles = filesUnder(srcDir).filter((path) =>
  path.endsWith(".stories.tsx"),
);

describe("the catalog indexes this directory", () => {
  // A census that reads a smaller tree reports the same word a clean one does.
  // Both floors are well under today's counts and exist to fail when the walk
  // breaks, not to pin a number somebody must maintain.
  it("reads the tree it is pointed at", () => {
    expect(primitiveModules.length).toBeGreaterThan(30);
    expect(storyFiles.length).toBeGreaterThan(100);
    expect(documentedRoots.size).toBeGreaterThan(4);
    expect(catalogTable.length).toBeGreaterThan(10_000);
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
    const lying = tableRows(catalogTable).flatMap((cells) => {
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

// A gate asserting a shape is ABSENT passes identically over a clean tree and
// over a detector that has stopped detecting. These plant each defect and read
// the detector directly.
describe("the detectors report what they are for", () => {
  const probe = join(dsDir, "probe.tsx");

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
});
