/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import ts from "typescript";
import { afterEach, describe, expect, it } from "vitest";
import { DESCRIPTION_PROPS } from "../../census/rules/panelhead.census";
import { parseSource, sourceFileAt } from "../../scripts/lib/source-tree";
import { withoutComments } from "../testing/css";
import { Card, SectionHeader } from "./atoms";
import { Panel, PanelBody, PanelGroupHead } from "./panel";

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
// This file holds the components: no head declares a description prop, and the
// title reads at one type at every level. census/rules/panelhead.census.ts
// holds the tree: no caller, class or sheet brings the slot back.

afterEach(cleanup);

const dsDir = dirname(fileURLToPath(import.meta.url));

// The four components a head is built from, and the file each lives in.
const HEAD_COMPONENTS: Readonly<Record<string, string>> = {
  Panel: join(dsDir, "panel.tsx"),
  PanelGroupHead: join(dsDir, "panel.tsx"),
  Card: join(dsDir, "atoms.tsx"),
  SectionHeader: join(dsDir, "atoms.tsx"),
};

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
      const props = declaredProps(sourceFileAt(file), component);
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

// The type size a heading's `data-size` stands for, read from the token sheet.
function sizeRem(size: string | undefined): number {
  const token = `--fontHeading${(size ?? "").replace(/^x*./, (lead) =>
    lead.toUpperCase(),
  )}`;
  const tokens = withoutComments(
    readFileSync(join(dsDir, "tokens.css"), "utf8"),
  );
  const declared = new RegExp(`${token}:[^;]*?([\\d.]+)rem\\s*/`).exec(tokens);
  if (!declared) throw new Error(`tokens.css declares no ${token}`);
  return Number(declared[1]);
}

// A group head labels the rows under it, at their size, so the pane keeps one
// title. At the title's size the groups read as panes inside a pane.
describe("a group head reads under the pane's title, at either level", () => {
  it.each([["h3" as const], ["h4" as const]])("draws %s at xsmall", (level) => {
    render(
      <Panel title="Usage">
        <PanelGroupHead title="Spend by task" level={level} id="spend" />
      </Panel>,
    );
    const group = headingAt("Spend by task");
    expect(group.tagName).toBe(level.toUpperCase());
    expect(group.id).toBe("spend");
    expect(group.dataset.size).toBe("xsmall");
    const pane = headingAt("Usage");
    expect(sizeRem(group.dataset.size)).toBeLessThan(
      sizeRem(pane.dataset.size),
    );
  });
});
