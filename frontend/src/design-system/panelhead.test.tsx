/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import ts from "typescript";
import { afterEach, describe, expect, it } from "vitest";
import { filesMatching, parseSource } from "../../scripts/lib/source-tree";
import { Card, SectionHeader } from "./atoms";
import { Panel, PanelBody } from "./panel";

// A PANEL OR CARD HEAD IS A TITLE AND, OPTIONALLY, THE VERBS THAT ACT ON IT.
// Never a description.
//
// The title names the thing and the body is the thing; a sentence wedged
// between them is a third voice saying what one of the other two already said,
// and the tree paid for it twice. It cost HEIGHT: a head that grows for a
// description is a head at two heights, so a column of panels went ragged and
// screens re-spaced the band one at a time to get it back. And it cost the
// TITLE: the type stepped down with the outline level, so the same head was
// drawn at three sizes depending on how deeply a caller had nested it.
//
// So the head's title is `Heading size="medium"`, and the level is a structural
// choice that picks the ELEMENT alone — the one exception being
// `SectionHeader level={1}`, which is not a shallower card head but the PAGE's
// own name and reads at the page-title size. Copy that genuinely adds something
// is body content, where it can be as long as it needs to be.
//
// Four arms, because the rule can be broken in four places:
//
//   type   — a `sub`/`description`/`intro` prop back on one of the four
//            components. The prop is the invitation; without it the rest cannot
//            happen.
//   call   — a caller passing one of those attributes. Held separately from the
//            type because a component spreading `...rest` would take the
//            attribute without ever declaring it.
//   sheet  — a stylesheet declaring the class such a slot would need. The class
//            outliving the slot is how the slot comes back: the next author
//            greps, finds a rule, and writes the span to match it.
//   render — the title's own type and element, read off the DOM at every level
//            the components offer, the page-naming level asserted beside its
//            neighbours so the two jobs cannot be levelled into one scale. The
//            first three are absence checks, and an absence check passes over a
//            tree that has quietly changed shape; this one says what the head
//            must positively look like.
//
// Under-recognition is the way a census like this fails silently — a walk that
// reads a smaller tree reports the same word, PASS — so both walks are derived
// from the tree, both fail closed under a floor, and each detector carries a
// planted case of the shape it claims to see.

afterEach(cleanup);

const dsDir = dirname(fileURLToPath(import.meta.url));
const srcDir = join(dsDir, "..");

// The four components a head is built from, and the file each lives in.
const HEAD_COMPONENTS: Readonly<Record<string, string>> = {
  Panel: join(dsDir, "panel.tsx"),
  PanelGroupHead: join(dsDir, "panel.tsx"),
  Card: join(dsDir, "atoms.tsx"),
  SectionHeader: join(dsDir, "atoms.tsx"),
};

// Three spellings of the same slot. A rule naming only `sub` would be answered
// by renaming the prop.
const DESCRIPTION_PROPS = ["sub", "description", "intro"];

// Floors under the two walks. The tree carries a thousand-odd components and
// nearly two hundred stylesheets; a corpus that comes back near these numbers
// means the walk broke rather than that the app shrank.
const TSX_FLOOR = 500;
const CSS_FLOOR = 50;

function tsxFiles(): readonly string[] {
  return filesMatching(srcDir, /\.tsx$/);
}

function cssFiles(): readonly string[] {
  return filesMatching(srcDir, /\.css$/);
}

// ---------------------------------------------------------------- the type

// The props a component declares, read off its parameter's type literal. A
// component whose parameter is not a literal object type returns `undefined`
// rather than an empty list, so "shape I cannot read" never reads as "declares
// nothing".
function declaredProps(
  source: ts.SourceFile,
  component: string,
): readonly string[] | undefined {
  let found: readonly string[] | undefined;
  const visit = (node: ts.Node): void => {
    if (
      ts.isFunctionDeclaration(node) &&
      node.name?.text === component &&
      node.parameters.length === 1
    ) {
      found = typeLiteralMembers(node.parameters[0].type);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// `Readonly<{ … }>` is how every component in this tree spells its props, so
// the members are one type argument in. A bare literal is read too, for a
// component that drops the wrapper.
function typeLiteralMembers(
  node: ts.TypeNode | undefined,
): readonly string[] | undefined {
  if (node === undefined) return undefined;
  if (ts.isTypeReferenceNode(node)) {
    return typeLiteralMembers(node.typeArguments?.[0]);
  }
  if (!ts.isTypeLiteralNode(node)) return undefined;
  return node.members.flatMap((member) =>
    ts.isPropertySignature(member) && ts.isIdentifier(member.name)
      ? [member.name.text]
      : [],
  );
}

describe("no head component declares a description slot", () => {
  it("reads the props off a component that has them", () => {
    const props = declaredProps(
      parseSource(
        "fixture.tsx",
        `export function Panel({ title }: Readonly<{ title?: string; sub?: string }>) {
           return <div>{title}</div>;
         }`,
      ),
      "Panel",
    );
    expect(props).toEqual(["title", "sub"]);
  });

  it.each(Object.entries(HEAD_COMPONENTS))(
    "%s declares a title and no sentence under it",
    (component, file) => {
      const props = declaredProps(
        parseSource(file, readFileSync(file, "utf8")),
        component,
      );
      // Fails closed: a component this gate cannot find, or whose props it
      // cannot read, is not a component it has cleared.
      expect(props, `${component} in ${file}`).toBeDefined();
      expect(props).toContain("title");
      expect(
        (props ?? []).filter((prop) => DESCRIPTION_PROPS.includes(prop)),
      ).toEqual([]);
    },
  );
});

// ---------------------------------------------------------------- the call

type Offence = Readonly<{ file: string; line: number; attribute: string }>;

// Every description attribute passed to a head component, anywhere in a tree.
function descriptionAttributes(source: ts.SourceFile, file: string): Offence[] {
  const found: Offence[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const tag = node.tagName.getText(source);
      if (tag in HEAD_COMPONENTS) {
        for (const property of node.attributes.properties) {
          if (!ts.isJsxAttribute(property)) continue;
          const attribute = property.name.getText(source);
          if (!DESCRIPTION_PROPS.includes(attribute)) continue;
          found.push({
            file,
            line:
              source.getLineAndCharacterOfPosition(property.getStart(source))
                .line + 1,
            attribute: `${tag} ${attribute}`,
          });
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

describe("nothing under src passes a description to a head", () => {
  // The planted case, in both shapes a caller writes: the one-line element and
  // the multi-line opening tag a formatter produces once the props no longer
  // fit. A detector that matched a line would see only the first.
  it("sees a description passed either way it is written", () => {
    const found = descriptionAttributes(
      parseSource(
        "fixture.tsx",
        `const a = <Card title="Passports" sub="One line." />;
         const b = (
           <Panel
             title="Purposes"
             description="Another line."
           >
             {null}
           </Panel>
         );
         const c = <SectionHeader title="Rooms" intro="A third." />;
         const d = <Panel title="Clean" titleAction={null}>{null}</Panel>;`,
      ),
      "fixture.tsx",
    );
    expect(found.map((offence) => offence.attribute)).toEqual([
      "Card sub",
      "Panel description",
      "SectionHeader intro",
    ]);
  });

  it("finds none in the tree", () => {
    const files = tsxFiles();
    expect(files.length).toBeGreaterThan(TSX_FLOOR);
    const offences = files.flatMap((file) =>
      descriptionAttributes(
        parseSource(file, readFileSync(file, "utf8")),
        file,
      ),
    );
    expect(
      offences.map((offence) => `${offence.file}:${offence.line}`),
    ).toEqual([]);
  });
});

// --------------------------------------------------------------- the sheet

// A class a head's description slot would need, by its name. `-head-sub` and
// `-title-sub` are the two ways the tree spelled one: the prefix is whichever
// surface grew it, so the rule is the SUFFIX rather than a list of prefixes.
const HEAD_SUB_CLASS = /\.[\w-]*-(?:head|title)-sub(?![\w-])/;

// The other shape: a generic `.sub` scoped INSIDE a head. It needs no name of
// its own, which is exactly why a name-only detector would miss it.
const SUB_IN_HEAD = /\.(?:panel-head|section-header)\b[^,{}]*\s\.sub(?![\w-])/;

function headSubSelectors(css: string): readonly string[] {
  return [...css.matchAll(/([^{}]+)\{[^{}]*\}/g)]
    .flatMap(([, selectorList]) => selectorList.split(","))
    .map((selector) => selector.trim().replace(/\s+/g, " "))
    .filter(
      (selector) => HEAD_SUB_CLASS.test(selector) || SUB_IN_HEAD.test(selector),
    );
}

describe("no stylesheet declares a head description", () => {
  it("sees both shapes, and leaves a body class alone", () => {
    expect(
      headSubSelectors(`
        .panel-head-sub { color: var(--textSecondary); }
        .co-glance .settings-panel-title-sub { max-width: 78ch; }
        .section-header .sub { max-width: 78ch; }
        .panel-head .panel-title { color: var(--textPrimary); }
        .filter-tabs > .sub { display: block; }
        .rmap-panel-sub { margin: 0; }
      `),
    ).toEqual([
      ".panel-head-sub",
      ".co-glance .settings-panel-title-sub",
      ".section-header .sub",
    ]);
  });

  it("finds none in the tree", () => {
    const files = cssFiles();
    expect(files.length).toBeGreaterThan(CSS_FLOOR);
    const offences = files.flatMap((file) =>
      headSubSelectors(readFileSync(file, "utf8")).map(
        (selector) => `${file}: ${selector}`,
      ),
    );
    expect(offences).toEqual([]);
  });
});

// -------------------------------------------------------------- the render

// What the DOM must say, at every level the components offer. `data-size` is
// the hook the type sheet keys off, so reading it reads the type itself rather
// than a class that is supposed to carry it.
function headingAt(name: string): HTMLElement {
  return screen.getByRole("heading", { name });
}

describe("a head title reads at one size however deeply it sits", () => {
  // A panel is always a section of something, so both its levels are card
  // heads and both draw the same type. The level moves the element alone.
  it.each([
    [undefined, "H2"],
    [2 as const, "H2"],
    [3 as const, "H3"],
  ])("draws Panel titleLevel=%s as %s at medium", (level, tag) => {
    render(
      <Panel title="Passports" titleLevel={level}>
        <PanelBody>Two active.</PanelBody>
      </Panel>,
    );
    const title = headingAt("Passports");
    expect(title.tagName).toBe(tag);
    expect(title.dataset.size).toBe("medium");
    expect(title.classList.contains("panel-title")).toBe(true);
  });

  // `1` is the page's own NAME rather than a shallower card head — the only
  // thing naming a surface the app shell has yielded to — so it reads at the
  // page-title size. Asserted beside its neighbours, because the whole risk
  // here is the two jobs being read as one scale and levelled together.
  it.each([
    [1 as const, "H1", "large"],
    [2 as const, "H2", "medium"],
    [3 as const, "H3", "medium"],
  ])("draws Card level=%s as %s at %s", (level, tag, size) => {
    render(
      <Card title="Rooms" level={level}>
        <p>None yet.</p>
      </Card>,
    );
    const title = headingAt("Rooms");
    expect(title.tagName).toBe(tag);
    expect(title.dataset.size).toBe(size);
  });

  it("draws a bare SectionHeader the same way its card does", () => {
    render(<SectionHeader title="Purposes" level={3} />);
    const title = headingAt("Purposes");
    expect(title.tagName).toBe("H3");
    expect(title.dataset.size).toBe("medium");
  });
});
