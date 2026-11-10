// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { tableRows, tablesIn } from "../../scripts/lib/design-catalog";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import {
  catalogGroups,
  componentsIn,
  csfTitle,
  falseStoryClaims,
  rule,
} from "./catalog.census";

const dsDir = join(srcDir, "design-system");

// A gate asserting a shape is absent passes over a clean tree and over a
// detector that stopped detecting. These plant each defect.
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

  it("sees an export list's public name, not the local one", () => {
    // `export { Card as Plate }` is imported as Plate, the noun a reader greps.
    expect(
      componentsIn(
        probe,
        "const Card = () => <div />;\nexport { Card as Plate };",
      ),
    ).toEqual(["Plate"]);
  });

  it("does not see a re-export from another module", () => {
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

  it("holds a bare ✅ to a story that imports the row's module", () => {
    const stories: Record<string, string> = {
      "panel.stories.tsx": "export default {};",
      "button.stories.tsx": 'import { Button } from "./atoms";',
      "badge.stories.tsx": 'import { Badge } from "./badge-lookalike";',
      "card.stories.tsx": 'import type { CardProps } from "./atoms";',
      "field.stories.tsx": 'import { type FieldControl } from "./atoms";',
      "textarea.stories.tsx": 'import "./atoms";',
      "select.stories.tsx": 'import { type Option, Select } from "./atoms";',
      "heading.stories.tsx": 'import * as atoms from "./atoms";',
    };
    const read = (story: string) => stories[story] ?? null;
    const row = (primitive: string, file: string, story = "✅") => [
      ` ${primitive} `,
      " for ",
      ` ${file} `,
      ` ${story} `,
    ];
    expect(
      falseStoryClaims(
        [
          row("`Panel`", "`panel.tsx`"),
          row("**`Button`**", "`atoms.tsx`"),
          row("`Checkbox` / `Button`", "`atoms.tsx`"),
          row("`Badge`", "`atoms.tsx`"),
          row("`Kbd`", "`atoms.tsx`"),
          row("`useScrollRegion`", "`atoms.tsx`"),
          row("`Kbd`", "`atoms.tsx`", "✅ (`Button`)"),
          row("`.link-button`", "`atoms.css`"),
          row("`Card`", "`atoms.tsx`"),
          row("`Field`", "`atoms.tsx`"),
          row("`Textarea`", "`atoms.tsx`"),
          row("`Select`", "`atoms.tsx`"),
          row("`Heading`", "`atoms.tsx`"),
        ],
        read,
      ),
    ).toEqual([
      "`Badge` claims ✅ but no story imports atoms.tsx: badge.stories.tsx, atoms.stories.tsx",
      "`Kbd` claims ✅ but no story imports atoms.tsx: kbd.stories.tsx, atoms.stories.tsx",
      "`useScrollRegion` claims ✅ but no story imports atoms.tsx: atoms.stories.tsx",
      "`Card` claims ✅ but no story imports atoms.tsx: card.stories.tsx, atoms.stories.tsx",
      "`Field` claims ✅ but no story imports atoms.tsx: field.stories.tsx, atoms.stories.tsx",
      "`Textarea` claims ✅ but no story imports atoms.tsx: textarea.stories.tsx, atoms.stories.tsx",
    ]);
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
});

describe("the catalog rule", () => {
  it("reads a story's title at its default export, not in its fixtures", () => {
    const story = (text: string) =>
      csfTitle(parseSource(join(dsDir, "x.stories.tsx"), text));
    expect(
      story(
        'const doc = { title: "Terms v4" };\nexport default { title: "Components/X" };',
      ),
    ).toBe("Components/X");
    expect(story("export default {};")).toBeNull();
  });

  it("reports a planted unnamed component through its own corpus", () => {
    const path = join(dsDir, "plantedwidget.tsx");
    const text = "export function PlantedWidgetXyz() {\n  return <div />;\n}\n";
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toContain(
      "src/design-system/plantedwidget.tsx exports PlantedWidgetXyz, and the catalog never names it",
    );
  });
});
