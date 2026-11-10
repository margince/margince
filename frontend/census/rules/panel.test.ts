// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir } from "../tree";
import {
  bandRules,
  PANEL_SHEET,
  rule,
  stateArmRoleInsets,
  titleTypeRules,
} from "./panel.census";

const panelCss = () => readFileSync(PANEL_SHEET, "utf8");

describe("panel.css is the only sheet that shapes the head band", () => {
  it("recognises the band where it is declared", () => {
    const own = bandRules(panelCss());
    expect(own.map((rule) => rule.selector)).toContain(".panel-head");
  });

  // No `.panel-head-*` class exists, so the fixture names one a future head
  // might add. A detector reading it as the band would fail a clean sheet.
  it("reads a rule about the head's content as content", () => {
    const inside = bandRules(`
      .ext-unit > .panel-head > .ext-unit-actions { flex: 0 1 auto; }
      .panel-head-count { margin-inline-start: var(--space-2); }
    `);
    expect(inside).toEqual([]);
  });

  it("reads a title rule as the title, wherever it hangs", () => {
    const css = `
      .co-glance-cols .panel > .panel-head .panel-title { font-family: var(--fontFamilyBody); }
      .rmap-panel-title { font-family: var(--fontFamilyBody); }
      .pe-memory .panel-head .panel-title:hover { color: var(--accent); }
    `;
    expect(bandRules(css)).toEqual([]);
    expect(titleTypeRules(css).map((rule) => rule.selector)).toEqual([
      ".co-glance-cols .panel > .panel-head .panel-title",
    ]);
  });

  it("reads a band rule wrapped in a query, and one that follows an import", () => {
    const wrapped = bandRules(`
      @import "./other.css";
      @media (max-width: 40rem) {
        .pe-memory .panel-head { min-height: 0; }
      }
      .co-glance .panel > .panel-head:has(.panel-title) { padding-block: 0; }
    `);
    expect(wrapped.map((rule) => rule.selector)).toEqual([
      ".pe-memory .panel-head",
      ".co-glance .panel > .panel-head:has(.panel-title)",
    ]);
  });
});

describe("the pane insets the state arm it has no body to wrap", () => {
  // The near misses. A rung is the spacing-roles gate's question. A part of an
  // arm is not the arm, and the primitive sizing itself is the owner.
  it("recognises a screen spelling the pane's inset for its own panel", () => {
    const found = stateArmRoleInsets(`
      .auto-inspector > .pending,
      .auto-inspector > .empty { padding: var(--padPanel); }
      .palette-list .pending { padding: var(--space-2); }
      .empty-plate { padding: var(--padCard); }
      .empty { padding: var(--space-5) var(--padCard); }
    `);
    expect(found.map((rule) => rule.selector)).toEqual([
      ".auto-inspector > .pending",
      ".auto-inspector > .empty",
    ]);
  });
});

describe("the panel rule", () => {
  const area: Area = "src";
  const planted = (path: string, text: string) =>
    plantedFindings(rule, [{ path, area, text }]);

  it("reports a screen re-spacing the band, its title, a state arm or the hyphen limit", () => {
    expect(
      planted(
        join(srcDir, "features", "planted.css"),
        [
          ".pe-memory .panel-head { min-height: 0; border-bottom: 0; }",
          ".pe-memory .panel-title { font-size: 1rem; }",
          ".pe-memory > .pending { padding: var(--padPanel); }",
          ":root { hyphenate-limit-chars: 6 3 3; }",
        ].join("\n"),
      ),
    ).toEqual([
      "features/planted.css: .pe-memory .panel-head sets min-height",
      "features/planted.css: .pe-memory .panel-head sets border-bottom",
      "features/planted.css: .pe-memory .panel-title sets font-size",
      "features/planted.css: .pe-memory > .pending spells a pane's inset on a state arm",
      "features/planted.css: sets hyphenate-limit-chars",
    ]);
  });

  it("lets panel.css shape its own band", () => {
    expect(planted(PANEL_SHEET, panelCss())).toEqual([]);
  });
});
