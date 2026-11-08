// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { srcDir } from "../tree";
import { glyphsInMenus, MENU } from "./overflowmenu-icons.census";

describe("an item in an overflow menu carries words, not glyphs", () => {
  it("names a menu the design system still exports", () => {
    // The subject, read off its owner. A rename the gate does not follow leaves
    // the census matching a tag nothing writes.
    const owner = readFileSync(
      join(srcDir, "design-system", "overflowmenu.tsx"),
      "utf8",
    );
    expect(
      new RegExp(`export function ${MENU}\\b`).test(owner),
      `${MENU} is not exported from design-system/overflowmenu.tsx — this gate watches a component that no longer exists`,
    ).toBe(true);
  });

  it("can see a glyph that is really there", () => {
    // The positive case, on planted source: the tree must not hold any of these
    // shapes, so nothing real can exercise them.
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
    // Nested to any depth, which is where glyphs sit: an item is a Button and the
    // glyph is inside it.
    expect(
      found(`<${MENU}>{ok ? <span><em><Mail /></em></span> : null}</${MENU}>`),
    ).toEqual(["`Mail` is a glyph from lucide-react"]);
    // An item of words only is not a finding.
    expect(found(`<${MENU}><Button>Write</Button></${MENU}>`)).toEqual([]);
    // Nor is a glyph outside the menu, where the header's icon-only verbs and
    // the menu's own trigger live.
    expect(
      found(`<div><Mail /><${MENU}><Button>Write</Button></${MENU}></div>`),
    ).toEqual([]);
  });
});
