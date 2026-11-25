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
const bare = (selector: string) =>
  `${SHEET}: ${selector} draws no edge of its own and names no field or ruled row around it`;

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
        markup(
          '<input className="box" />',
          '<textarea className="richtext" />',
        ),
        [
          ".shell:focus-within { border-color: var(--accent); }",
          ".shell > .box { border: 0; }",
          ".shell .box { border-color: transparent; }",
          ".loose .box { border: none; }",
          ".box { border: 1px solid transparent; }",
          ".compose-sheet > .richtext { border-color: transparent; }",
        ],
      ),
    ).toEqual([bare(".loose .box"), bare(".box")]);
  });

  it("passes no edge right on a row ruled solid in the control edge, at rest", () => {
    const control = "1px solid var(--borderControl)";
    const mixed = "color-mix(in srgb, var(--borderControl) 50%, transparent)";
    const rows = [
      `.ruled { border-bottom: ${control}; }`,
      `.boxed { border: ${control}; }`,
      `.foot { border-block-end: ${control}; }`,
      `.faint { border-bottom: ${faint}; }`,
      `.mixed { border-bottom: 1px solid ${mixed}; }`,
      `.lit:hover { border-bottom: ${control}; }`,
      `.top { border-top: ${control}; }`,
      ".legend { border-block-start: 2px dashed var(--borderControl); }",
      ".dashed { border-bottom: 1px dashed var(--borderControl); }",
    ];
    const passing = [
      ".ruled .box",
      ".boxed > .box",
      ".foot .box",
      ".page .ruled .box",
      ".ruled > .wrap .box",
    ];
    const failing = [
      ".faint .box",
      ".mixed .box",
      ".lit .box",
      ".top .box",
      ".legend .box",
      ".dashed .box",
      ".ruled .x .y .box",
      ".ruled + .box",
    ];
    expect(
      findings(markup('<input className="box" />'), [
        ...rows,
        ...[...passing, ...failing].map(
          (selector) => `${selector} { border-color: transparent; }`,
        ),
      ]),
    ).toEqual(failing.map(bare));
  });

  it("keeps a row's or a field's context with it", () => {
    expect(
      findings(markup('<input className="box" />'), [
        ".bar[data-x] .row { border-bottom: 1px solid var(--borderControl); }",
        ".bar[data-x] .row .box { border: 0; }",
        ".row .box { border: 0; }",
        ".bar[data-x] .field:focus-within { border-color: var(--accent); }",
        ".bar[data-x] .field > .box { border: 0; }",
        ".field > .box { border: 0; }",
        `.field { border: ${faint}; }`,
        `.bar[data-x] .field { border: ${faint}; }`,
        ".p + .q { border-bottom: 1px solid var(--borderControl); }",
        ".p + .q .box { border: 0; }",
        ".p .q .box { border: 0; }",
      ]),
    ).toEqual([
      bare(".row .box"),
      bare(".field > .box"),
      `${SHEET}: .bar[data-x] .field draws ${faint}`,
      bare(".p .q .box"),
    ]);
  });

  it("reads a box lit in context as a field there, its own edge included", () => {
    expect(
      findings(markup('<input className="box" />'), [
        ".a .b:focus-within { border-color: var(--accent); }",
        `.a .b { border: ${faint}; }`,
        ".a .b { border: 0; }",
        ".a .b .box { border: 0; }",
        ".c:focus-within .d:focus-within { border-color: var(--accent); }",
        ".c .d > .box { border: 0; }",
      ]),
    ).toEqual([`${SHEET}: .a .b draws ${faint}`, bare(".a .b")]);
  });

  it("unseats a row that any rule on its box takes the foot from", () => {
    expect(
      findings(markup('<input className="box" />'), [
        ".row { border-bottom: 1px solid var(--borderControl); }",
        ".row.flat { border-bottom: 0; }",
        ".row .box { border: 0; }",
        ".row.flat .box { border: 0; }",
        ".kept { border-bottom: 1px solid var(--borderControl); }",
        ".kept:hover { border-bottom-color: var(--accent); }",
        ".kept .box { border: 0; }",
      ]),
    ).toEqual([bare(".row .box"), bare(".row.flat .box")]);
  });

  it("passes a field that clears its sides and rules its foot in the control edge", () => {
    expect(
      findings(markup('<input className="box" />'), [
        ".ruled .box { border-color: transparent; border-block-end-color: var(--borderControl); }",
        ".faint .box { border-color: transparent; border-block-end-color: var(--borderSubtle); }",
        ".zero .box { border: 0; border-block-end-color: var(--borderControl); }",
        ".top .box { border-color: transparent; border-top-color: var(--borderControl); }",
        ".side .box { border: 1px solid transparent; border-left: 1px solid var(--borderControl); }",
        ".dotted .box { border-color: transparent; border-bottom: 1px dotted var(--borderControl); }",
        ".thin .box { border-color: transparent; border-bottom-color: var(--borderControl); border-bottom-width: 0; }",
      ]),
    ).toEqual([
      bare(".faint .box"),
      `${SHEET}: .faint .box draws var(--borderSubtle)`,
      ...[".zero", ".top", ".side", ".dotted", ".thin"].map((row) =>
        bare(`${row} .box`),
      ),
    ]);
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
