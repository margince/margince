// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./overlayscrim.census";

const path = join(srcDir, "planted.css");
const planted = (text: string) =>
  plantedFindings(rule, [{ path, area: "src", text }]);

describe("a layer that dims the page", () => {
  it("passes the scrim token and a layer that does not dim", () => {
    expect(
      planted(
        ".overlay { background: var(--overlayScrim); } .overlay:has(.full) { background: none; }",
      ),
    ).toEqual([]);
  });

  it("reports a scrim, overlay or backdrop that paints its own colour", () => {
    expect(
      planted(
        ".railscrim { background: color-mix(in srgb, var(--textPrimary) 38%, transparent); } dialog::backdrop { background-color: #0008; }",
      ),
    ).toEqual([
      `src/planted.css: .railscrim paints color-mix(in srgb, var(--textPrimary) 38%, transparent)`,
      `src/planted.css: dialog::backdrop paints #0008`,
    ]);
  });

  it("reads the subject, not a parent that names an overlay", () => {
    expect(
      planted(".overlay .modal { background: var(--bgElevated); }"),
    ).toEqual([]);
  });
});
