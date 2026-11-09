// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import type { Rule } from "../../scripts/lib/css-rules";
import { type Area, frontendRoot, plantedFindings } from "../tree";
import { accentStates, rule, unexcused } from "./selectedstate.census";

describe("a selected or pressed state is neutral", () => {
  it("fails a new state selector the tree does not have yet", () => {
    const probe = (selector: string, body: string): Rule[] => [
      { file: join(frontendRoot, "src", "probe.css"), selector, body },
    ];
    expect(
      accentStates(
        probe(
          '.steps a[aria-current="step"]',
          "background: var(--accentLight)",
        ),
      ),
    ).toHaveLength(1);
    expect(
      accentStates(probe(".tabs .tab.on .glyph", "color: var(--accentText)")),
    ).toHaveLength(1);
    expect(
      accentStates(
        probe(".row.is-selected", "box-shadow: inset 3px 0 var(--accent)"),
      ),
    ).toHaveLength(1);
    expect(
      accentStates(probe(".row:not(.on)", "color: var(--accentText)")),
    ).toEqual([]);
    expect(
      accentStates(probe(".row.on", "outline-color: var(--accent)")),
    ).toEqual([]);
  });

  it("lets through only the rail's own tint and ink, in its own sheet", () => {
    const shell = join(frontendRoot, "src", "app", "shell.css");
    const elsewhere = join(frontendRoot, "src", "probe.css");
    const marked = (file: string, selector: string, body: string) =>
      unexcused([{ file, selector, body }]);
    const tint = "background: var(--accentLight)";
    expect(marked(shell, ".rail .navitem.active", tint)).toEqual([]);
    expect(
      marked(shell, ".rail .navitem.active", "color: var(--accentText)"),
    ).toEqual([]);
    expect(
      marked(shell, ".rail .navitem.active", "background: var(--accent)"),
    ).toHaveLength(1);
    expect(marked(shell, ".tabs .tab.active", tint)).toHaveLength(1);
    expect(marked(elsewhere, ".rail .navitem.active", tint)).toHaveLength(1);
  });

  it("reports a planted accent state through its own corpus", () => {
    const area: Area = "src";
    expect(
      plantedFindings(rule, [
        {
          path: join(frontendRoot, "src", "features", "planted.css"),
          area,
          text: ".tabs .tab.on { color: var(--accentText); }",
        },
      ]),
    ).toEqual([
      "src/features/planted.css: .tabs .tab.on { color: var(--accentText) }",
    ]);
  });
});
