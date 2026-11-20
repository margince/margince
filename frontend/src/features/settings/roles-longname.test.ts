// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// A role name is typed by an admin and may hold no space, so the drawer's title
// and the role row both break it anywhere. jsdom lays nothing out, so the
// rules are read from the sheets.
function ruleOf(sheet: string, selector: string): string {
  const css = readFileSync(new URL(sheet, import.meta.url), "utf8");
  const at = css.indexOf(`\n${selector} {`);
  if (at === -1) throw new Error(`${sheet} has no rule for ${selector}`);
  return css.slice(at, css.indexOf("}", at));
}

describe("a long role name", () => {
  it("wraps in a dialog's title", () => {
    expect(ruleOf("../../design-system/atoms.css", ".modal-title")).toContain(
      "overflow-wrap: anywhere",
    );
  });

  it("wraps in the role row", () => {
    expect(
      ruleOf("./roles-settings.css", ".roles-row-press > .cell-stack"),
    ).toContain("overflow-wrap: anywhere");
  });
});
