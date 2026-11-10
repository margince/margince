// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { rule, type TitledCard, titledCardsIn } from "./cardzones.census";

// The detector must see each shape, or a clean tree and a blind reader look
// the same.
describe("the detector", () => {
  const read = (source: string): TitledCard[] =>
    titledCardsIn("fixture.tsx", parseSource("fixture.tsx", source));

  it("reads the prop form, open and self-closing", () => {
    expect(read('<Card title={t("consent.title")}>{rows}</Card>')).toHaveLength(
      1,
    );
    expect(read("<Card title={title} sub={sub} />")).toHaveLength(1);
  });

  it("reads the hand-rolled head, direct and under a padding wrapper", () => {
    expect(read("<Card><SectionHeader title={title} /></Card>")).toHaveLength(
      1,
    );
    expect(
      read(
        '<Card>\n  <div style={{ padding: "var(--padCard)" }}>\n    <SectionHeader title={title} />\n  </div>\n  {rows}\n</Card>',
      ),
    ).toHaveLength(1);
  });

  it("reads a head drawn under a condition", () => {
    expect(
      read("<Card>{visible && <SectionHeader title={title} />}</Card>"),
    ).toHaveLength(1);
  });

  // A guard wraps its head in a fragment once the head grows a sibling.
  it("reads a head a condition wraps in a fragment", () => {
    expect(
      read(
        "<Card>{visible && <><SectionHeader title={title} />{rows}</>}</Card>",
      ),
    ).toHaveLength(1);
    expect(
      read(
        "<Card>{visible ? <><SectionHeader title={title} /></> : null}</Card>",
      ),
    ).toHaveLength(1);
    expect(
      read(
        '<Card>\n  <div style={{ padding: "var(--padCard)" }}>\n    {visible && <><SectionHeader title={title} /></>}\n  </div>\n</Card>',
      ),
    ).toHaveLength(1);
  });

  it("leaves an untitled Card alone", () => {
    expect(read('<Card as="li" className="grant-row">{name}</Card>')).toEqual(
      [],
    );
    expect(read("<Card inset>{body}</Card>")).toEqual([]);
  });

  it("does not read a Panel's title as a Card's", () => {
    expect(read("<Panel title={title}>{body}</Panel>")).toEqual([]);
  });

  it("credits a nested surface's head to the nested surface", () => {
    const nested = read(
      "<Card>\n  <Card>\n    <SectionHeader title={title} />\n  </Card>\n</Card>",
    );
    expect(nested).toHaveLength(1);
    expect(nested[0].line).toBe(2);
  });

  it("names the line the card opens on", () => {
    expect(read("<div>\n  <Card title={title} />\n</div>")[0].line).toBe(2);
  });
});

describe("the cardzones rule", () => {
  it("reports a planted titled card through its own corpus and across", () => {
    const path = join(srcDir, "features", "planted", "zone.tsx");
    const text = "export const F = () => <Card title={title} />;\n";
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([
      "features/planted/zone.tsx:1 <Card title=…> is a titled zone — use Panel (design-system/panel.tsx)",
      "features/contacts/contactnetwork/edgedetail.tsx: stale waiver: remove the entry",
    ]);
  });
});
