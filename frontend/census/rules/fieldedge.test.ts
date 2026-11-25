// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir, treeOf } from "../tree";
import { rule } from "./fieldedge.census";

const area: Area = "src";
const at = (name: string) => join(srcDir, "features", name);
const SHEET = "src/features/planted.css";

const planted = (tsx: string, css: readonly string[]) => [
  { path: at("planted.tsx"), area, text: tsx },
  { path: at("planted.css"), area, text: css.join("\n") },
];
const findings = (tsx: string, css: readonly string[]) =>
  plantedFindings(rule, planted(tsx, css));
const markup = (...lines: string[]) =>
  ["export const F = () => (", "  <>", ...lines, "  </>", ");"].join("\n");
const faint = "1px solid var(--borderSubtle)";

describe("a text field's resting edge", () => {
  it("reports a field written on an <input> that rests on a faint edge", () => {
    expect(
      findings(markup('<input className="box" />'), [
        `.box { border: ${faint}; }`,
      ]),
    ).toEqual([`${SHEET}: .box draws ${faint}`]);
  });

  it("reads a class a variable holds, a <select>, and an editable box", () => {
    const tsx = [
      'const named = "box";',
      "export const F = () => (",
      "  <>",
      "    <input className={named} />",
      '    <select className="pick" />',
      '    <div contentEditable className="editor" />',
      '    <div role="textbox" className="typed" />',
      '    <div role="textbox" aria-readonly="true" className="shown" />',
      "  </>",
      ");",
    ].join("\n");
    expect(
      findings(
        tsx,
        [".box", ".pick", ".editor", ".typed", ".shown"].map(
          (name) => `${name} { border: ${faint}; }`,
        ),
      ),
    ).toEqual(
      [".box", ".pick", ".editor", ".typed"].map(
        (name) => `${SHEET}: ${name} draws ${faint}`,
      ),
    );
  });

  it("leaves out a toggle however its type is written", () => {
    expect(
      findings(
        markup(
          '<input type={"checkbox"} className="tick" />',
          '<input type="radio" className="dot" />',
        ),
        [`.tick, .dot { border: ${faint}; }`],
      ),
    ).toEqual([]);
  });

  it("reports a shell that shows the caret and every class joined to it", () => {
    expect(
      findings("", [
        ".tall { border-color: var(--groundEdge); }",
        ".wide.tall { border-color: var(--borderSubtle); }",
        ".shell.wide { border-color: var(--borderSubtle); }",
        ".shell:focus-within { border-color: var(--accent); }",
      ]),
    ).toEqual([
      `${SHEET}: .tall draws var(--groundEdge)`,
      `${SHEET}: .wide.tall draws var(--borderSubtle)`,
      `${SHEET}: .shell.wide draws var(--borderSubtle)`,
    ]);
  });

  it("does not widen the fields through a state class", () => {
    expect(
      findings("", [
        ".shell:focus { border-color: var(--accent); }",
        ".shell.is-busy { border-color: var(--borderStrong); }",
        `.is-busy { border: ${faint}; }`,
      ]),
    ).toEqual([]);
  });

  it("reads each alternative of :is(), and a rest narrowed by :not()", () => {
    expect(
      findings(markup('<input className="box" />'), [
        `:is(.box, .card) { border: ${faint}; }`,
        `.box:not(:disabled) { border: ${faint}; }`,
      ]),
    ).toEqual([
      `${SHEET}: .box draws ${faint}`,
      `${SHEET}: .box:not(:disabled) draws ${faint}`,
    ]);
  });

  it("reads logical sides, inset rings and drawn outlines", () => {
    expect(
      findings(markup('<input className="box" />'), [
        `.box { border-block-end: ${faint}; }`,
        ".box { border-inline-start-color: var(--borderStrong); }",
        ".box { box-shadow: var(--shadow-well), inset 0 0 0 1px var(--borderSubtle); }",
        ".box { outline: 1px solid var(--borderStrong); }",
        ".box { outline: 2px solid transparent; box-shadow: var(--shadow-well); }",
      ]),
    ).toEqual([
      `${SHEET}: .box draws ${faint}`,
      `${SHEET}: .box draws var(--borderStrong)`,
      `${SHEET}: .box rings itself in var(--borderSubtle)`,
      `${SHEET}: .box rings itself in var(--borderStrong)`,
    ]);
  });

  it("takes the control edge whole, not a mix that names it", () => {
    const mixed =
      "1px solid color-mix(in srgb, var(--borderControl) 50%, transparent)";
    expect(
      findings(markup('<input className="box" />'), [
        ".box { border: 1px solid var(--borderControl); }",
        ".box { border-color: var(--borderControl); }",
        `.box { border: ${mixed}; }`,
      ]),
    ).toEqual([`${SHEET}: .box draws ${mixed}`]);
  });

  it("passes no edge only inside a field, or where the exception names it", () => {
    expect(
      findings(
        markup('<input className="box" />', '<input className="input" />'),
        [
          ".shell:focus-within { border-color: var(--accent); }",
          ".shell > .box { border: 0; }",
          ".shell .box { border-color: transparent; }",
          ".loose .box { border: none; }",
          ".box { border: 1px solid transparent; }",
          ".palette-input input { border-color: transparent; }",
        ],
      ),
    ).toEqual([
      `${SHEET}: .loose .box draws no edge of its own and names no field or ruled row around it`,
      `${SHEET}: .box draws no edge of its own and names no field or ruled row around it`,
    ]);
  });

  it("passes no edge on a row ruled whole in the control edge, at rest", () => {
    const bare = (row: string) => `.${row} .box { border-color: transparent; }`;
    const mixed = "color-mix(in srgb, var(--borderControl) 50%, transparent)";
    expect(
      findings(markup('<input className="box" />'), [
        ".ruled { border-bottom: 1px solid var(--borderControl); }",
        `.faint { border-bottom: ${faint}; }`,
        `.mixed { border-bottom: 1px solid ${mixed}; }`,
        ".lit:hover { border-bottom: 1px solid var(--borderControl); }",
        ...["ruled", "faint", "mixed", "lit"].map(bare),
      ]),
    ).toEqual(
      ["faint", "mixed", "lit"].map(
        (row) =>
          `${SHEET}: .${row} .box draws no edge of its own and names no field or ruled row around it`,
      ),
    );
  });

  it("leaves out a button, a lit tile, a field's states and a shared class", () => {
    expect(
      findings(
        markup('<input className="shared" />', '<div className="shared" />'),
        [
          ".chip:focus-visible { border-color: var(--accent); }",
          `.chip { border: ${faint}; }`,
          ".tile:hover, .tile:focus-within { border-color: var(--accent); }",
          ".tile { border: 1px solid var(--groundEdge); }",
          `.shared { border: ${faint}; }`,
          ".row:not(:focus-within) { border-color: var(--borderSubtle); }",
          `input[type='radio'] { border: ${faint}; }`,
          ".input:hover { border-color: var(--borderSubtle); }",
          ".input[aria-invalid='true'] { border-color: var(--danger); }",
          `.input::placeholder { border: ${faint}; }`,
        ],
      ),
    ).toEqual([]);
  });
});

describe("the field census's floors", () => {
  it("fires when the markup reading finds no field", () => {
    const tree = treeOf(
      planted("", [
        ".input:focus, .textarea:focus { border-color: var(--accent); }",
        ".field-shell:focus-within, .lt-fsearch-input:focus { border-color: var(--accent); }",
        ".token-box { border: 0; }",
      ]),
    );
    expect(rule.floors?.(rule.corpus(tree), tree)).toEqual([
      ".token-box no longer reads as a field",
    ]);
  });
});
