// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { type Area, plantedFindings, srcDir } from "../tree";
import {
  descriptionAttributes,
  headSubSelectors,
  rule,
  subClassLines,
} from "./panelhead.census";

describe("nothing passes a description to a head", () => {
  // The one-line element and the multi-line opening tag a formatter writes.
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
    );
    expect(found.map((offence) => offence.attribute)).toEqual([
      "Card sub",
      "Panel description",
      "SectionHeader intro",
    ]);
  });
});

describe("nothing wears the class that slot wore", () => {
  it("sees the class alone, beside another, and built in an expression", () => {
    const found = subClassLines(
      parseSource(
        "fixture.tsx",
        `const a = <p className="card-sub">One line.</p>;
         const b = <p className="card-sub sr-only">Another line.</p>;
         const c = <p className={classes("card-sub", tone)}>A third.</p>;
         const d = <span className="dcard-subject-label">A label.</span>;
         const e = <p className="card-subtitle">A subtitle.</p>;`,
      ),
    );
    expect(found).toEqual([1, 2, 3]);
  });
});

describe("no stylesheet declares a head description", () => {
  it("sees both shapes, and leaves a body class alone", () => {
    expect(
      headSubSelectors(`
        .panel-head-sub { color: var(--textSecondary); }
        .co-glance .settings-panel-title-sub { max-width: 78ch; }
        .section-header .sub { max-width: 78ch; }
        .auth-card .card-sub { margin-top: var(--space-2); }
        .panel-head .panel-title { color: var(--textPrimary); }
        .filter-tabs > .sub { display: block; }
        .rmap-panel-sub { margin: 0; }
        .dcard-subject-label { text-transform: uppercase; }
      `),
    ).toEqual([
      ".panel-head-sub",
      ".co-glance .settings-panel-title-sub",
      ".section-header .sub",
      ".auth-card .card-sub",
    ]);
  });
});

describe("the panelhead rule", () => {
  it("reports each arm through its own corpus", () => {
    const area: Area = "src";
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.tsx"),
          area,
          text: 'export const A = () => <Card title="T" sub="S" />;\nexport const B = () => <p className="card-sub" />;',
        },
        {
          path: join(srcDir, "features", "planted.css"),
          area,
          text: ".panel-head-sub { margin: 0; }",
        },
      ]),
    ).toEqual([
      "src/features/planted.tsx:1: passes Card sub to a head",
      "src/features/planted.tsx:2: wears card-sub",
      "src/features/planted.css: .panel-head-sub",
    ]);
  });
});
