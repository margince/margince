// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Overflow-menu gate: an item in a menu is a LINE OF WORDS.
//
// The menu exists for the verbs a reader rarely wants, which is exactly the set
// whose glyphs nobody has learnt. A square with an envelope on it is readable
// because every reader already knows that verb from the glyph; a square with a
// rotating arrow on it is a guess, and the menu is where such a verb comes to
// have a whole line to explain itself. Putting the glyph back on that line
// spends the line and explains nothing — worse, the glyphs arrived one at a
// time, so a panel ended up with three items wearing one and four not, which
// reads as two kinds of item where there is only one.
//
// The rule is therefore absolute rather than per-menu: every item inside any
// `OverflowMenu`, anywhere in the app, carries words and no glyph. Icon-only
// belongs OUTSIDE the menu and only where the glyph IS the verb — mail, phone,
// calendar — which is `IconAction`'s own contract (design-system/README.md).
//
// WHAT THIS SEES: a lucide glyph, a raw `<svg>`, or a `*Icon` component written
// inside a `<OverflowMenu>`'s own subtree, in the file that writes it.
//
// WHAT IT CANNOT SEE, and this is the honest half: a component boundary. A menu
// whose child is `<ArchiveAction />` is a menu this gate reads as empty, and the
// glyph could be inside that component. Following it would mean resolving every
// component in the tree, and the same walk would then have to decide which of a
// component's branches actually render under a menu. What covers that instead is
// the rendered panel: each menu's screen has a story, and `make fe-uat` draws
// them. So this gate holds the shape an author types, and the stories hold the
// shape a reader sees.

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesUnder, parseSource } from "../../scripts/lib/source-tree";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const srcDir = join(frontendRoot, "src");

// The menu this gate is about, named once. Checked against the design system
// below, so a rename fails loudly rather than quietly emptying the census.
const MENU = "OverflowMenu";
const GLYPH_PACKAGE = "lucide-react";

/** Every local name in this file that resolves to a glyph from the icon set. */
function glyphImports(source: ts.SourceFile): Set<string> {
  const names = new Set<string>();
  for (const statement of source.statements) {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier) ||
      statement.moduleSpecifier.text !== GLYPH_PACKAGE
    ) {
      continue;
    }
    const clause = statement.importClause;
    if (!clause) {
      continue;
    }
    // `import Mail from` and `import * as Icons from` both put a glyph within
    // reach under one name, and the tag then reads `Mail` or `Icons.Mail`; the
    // root of a qualified tag is what is matched below, so one set holds both.
    if (clause.name) {
      names.add(clause.name.text);
    }
    if (clause.namedBindings) {
      if (ts.isNamespaceImport(clause.namedBindings)) {
        names.add(clause.namedBindings.name.text);
      } else {
        for (const element of clause.namedBindings.elements) {
          names.add(element.name.text);
        }
      }
    }
  }
  return names;
}

function isElement(
  node: ts.Node,
): node is ts.JsxElement | ts.JsxSelfClosingElement {
  return ts.isJsxElement(node) || ts.isJsxSelfClosingElement(node);
}

function tagOf(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  source: ts.SourceFile,
): string {
  const tag = ts.isJsxElement(node)
    ? node.openingElement.tagName
    : node.tagName;
  return tag.getText(source);
}

/**
 * Why this tag is a glyph rather than a word, or undefined when it is neither.
 *
 * The three answers are the three ways a glyph reaches the markup, and each is
 * named in the finding: an author who reads "an `<svg>`" and an author who
 * reads "a lucide glyph" are looking for different lines.
 */
function glyphReason(tag: string, glyphs: ReadonlySet<string>): string {
  // `Icons.Mail` is the namespace form; the root is what the import bound.
  const root = tag.split(".")[0];
  const leaf = tag.split(".").at(-1) ?? tag;
  if (glyphs.has(root)) {
    return `\`${tag}\` is a glyph from ${GLYPH_PACKAGE}`;
  }
  if (tag === "svg") {
    return "an `<svg>` is drawn here";
  }
  if (/Icon$/.test(leaf) && leaf !== "Icon") {
    return `\`${tag}\` is named as a glyph`;
  }
  return "";
}

type Finding = Readonly<{ file: string; line: number; why: string }>;

/**
 * Every glyph written inside an overflow menu in one file.
 *
 * Taken as TEXT rather than read from the path, so the fixture cases below run
 * the same function the tree does: a census that cannot demonstrate it sees the
 * defect has only demonstrated that it saw nothing.
 */
function glyphsInMenus(file: string, text: string): Finding[] {
  const source = parseSource(file, text);
  const glyphs = glyphImports(source);
  const findings: Finding[] = [];

  const inspect = (node: ts.Node) => {
    if (isElement(node)) {
      const why = glyphReason(tagOf(node, source), glyphs);
      if (why) {
        const { line } = source.getLineAndCharacterOfPosition(
          node.getStart(source),
        );
        findings.push({ file, line: line + 1, why });
      }
    }
    ts.forEachChild(node, inspect);
  };

  const visit = (node: ts.Node) => {
    if (ts.isJsxElement(node) && tagOf(node, source) === MENU) {
      // The children only: the menu's own trigger draws the ellipsis, which is
      // the one glyph on the surface that IS the control rather than an item.
      for (const child of node.children) {
        inspect(child);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return findings;
}

describe("an item in an overflow menu carries words, not glyphs", () => {
  // Stories and tests included, and no prefilter in front of the walk. A menu
  // written in a story is a menu a reader is shown, and a skip-list is the one
  // way this gate can fail short: it reads a smaller tree, finds nothing, and
  // reports the same word a clean tree does.
  const surfaces = filesUnder(srcDir).filter((file) => /\.tsx$/.test(file));

  it("names a menu the design system still exports", () => {
    // The subject, read off its owner. Renaming the component without renaming
    // it here leaves the walk below matching a tag nothing writes any more —
    // a green gate over a tree it never looked at.
    const atoms = readFileSync(
      join(srcDir, "design-system", "atoms.tsx"),
      "utf8",
    );
    expect(
      new RegExp(`export function ${MENU}\\b`).test(atoms),
      `${MENU} is not exported from design-system/atoms.tsx — this gate watches a component that no longer exists`,
    ).toBe(true);
  });

  it("can see a glyph that is really there", () => {
    // The positive case, against fixtures rather than the tree: every shape
    // below is one the tree is supposed NOT to contain, so nothing real can
    // exercise them. Without this the whole file passes by finding nothing.
    const found = (markup: string) =>
      glyphsInMenus(
        "fixture.tsx",
        `import { Mail } from "lucide-react";\nimport * as Icons from "lucide-react";\nexport const F = () => (${markup});\n`,
      ).map((finding) => finding.why);

    expect(found(`<${MENU}><Button><Mail />Write</Button></${MENU}>`)).toEqual([
      "`Mail` is a glyph from lucide-react",
    ]);
    expect(
      found(`<${MENU}><Button><Icons.Mail />Write</Button></${MENU}>`),
    ).toEqual(["`Icons.Mail` is a glyph from lucide-react"]);
    expect(found(`<${MENU}><Button><svg /> Write</Button></${MENU}>`)).toEqual([
      "an `<svg>` is drawn here",
    ]);
    expect(
      found(`<${MENU}><Button><WriteIcon /> Write</Button></${MENU}>`),
    ).toEqual(["`WriteIcon` is named as a glyph"]);
    // Nested to any depth, which is where the glyphs actually sit: an item is a
    // Button and the glyph is inside it, never a direct child of the menu.
    expect(
      found(`<${MENU}>{ok ? <span><em><Mail /></em></span> : null}</${MENU}>`),
    ).toEqual(["`Mail` is a glyph from lucide-react"]);
    // And the words-only item, which is the whole point, is not a finding.
    expect(found(`<${MENU}><Button>Write</Button></${MENU}>`)).toEqual([]);
    // Nor is a glyph OUTSIDE the menu — the header's icon-only verbs and the
    // menu's own trigger both live there.
    expect(
      found(`<div><Mail /><${MENU}><Button>Write</Button></${MENU}></div>`),
    ).toEqual([]);
  });

  it("reads the whole tree, and finds menus in it", { timeout: 60_000 }, () => {
    // Two floors, because either can fall while the other holds: a walk that
    // collected no files and a detector that matched no menus print the same
    // clean verdict as a tree that is genuinely clean.
    expect(surfaces.length).toBeGreaterThan(100);
    const menus = surfaces.filter((file) =>
      readFileSync(file, "utf8").includes(`<${MENU}`),
    );
    expect(
      menus.length,
      `no <${MENU}> was found anywhere under src/ — the detector is dark`,
    ).toBeGreaterThan(5);
  });

  it("holds every menu in the tree", { timeout: 60_000 }, () => {
    const findings = surfaces
      .flatMap((file) => glyphsInMenus(file, readFileSync(file, "utf8")))
      .map(
        (finding) =>
          `${relative(frontendRoot, finding.file)}:${finding.line}: ${finding.why}`,
      );

    expect(
      findings,
      `Items in an overflow menu carry words, not glyphs. Drop the glyph and ` +
        `keep the label; a verb that needs a glyph to be understood needs a ` +
        `sentence more, and the menu line is where it has room for one. ` +
        `Icon-only lives outside the menu, and only where the glyph IS the ` +
        `verb — see IconAction in design-system/README.md.\n${findings.join("\n")}\n`,
    ).toEqual([]);
  });
});
