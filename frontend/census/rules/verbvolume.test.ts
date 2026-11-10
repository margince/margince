// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { rule, secondMarks } from "./verbvolume.census";

const marks = (markup: string) =>
  secondMarks(parseSource("planted.tsx", `const a = (${markup});`));

describe("an AI verb is handed no second mark", () => {
  it("sees a Sparkles inside an ai, aiQuiet or computed variant", () => {
    expect(marks(`<Button variant="ai"><Sparkles />Draft</Button>`)).toEqual([
      1,
    ]);
    expect(
      marks(`<Button variant={"aiQuiet"}><span><Sparkles /></span></Button>`),
    ).toEqual([1]);
    expect(marks(`<Button variant={kind}><Sparkles /></Button>`)).toEqual([1]);
  });

  it("leaves a plain variant and a Sparkles outside a button alone", () => {
    expect(marks(`<Button variant="primary"><Sparkles /></Button>`)).toEqual(
      [],
    );
    expect(
      marks(`<div><Sparkles /><Button variant="ai">x</Button></div>`),
    ).toEqual([]);
  });

  it("reports a planted second mark through its own corpus and visit", () => {
    const text = `const a = (\n<Button variant="ai"><Sparkles /></Button>);\n`;
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "planted.tsx"), area: "src", text },
        { path: join(srcDir, "features", "planted.ts"), area: "src", text },
      ]),
    ).toEqual(["src/features/planted.tsx:2"]);
  });
});
