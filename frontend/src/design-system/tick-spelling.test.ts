// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Tick gate: `Checkbox` and `Radio` in atoms.tsx are the product's ONLY tick
// controls.
//
// A tick is four decisions — the box's size, its accent, its focus ring, and
// the words being the other half of the click target — and every surface that
// drew its own answered some of them and forgot the rest. The tree carried a
// `<button>` with a lucide `Check` in a bordered square, a `<li>` with
// `role="menuitemradio"` and a hand-drawn tick column, two "discs" that hid a
// native input behind a glyph, and half a dozen bare `<input type="checkbox">`
// each with its own wrapper class and its own idea of the gap. They looked
// alike and behaved differently: one lost the keyboard entirely, two announced
// a state nothing set, and the boxes came out at 13px beside 16px on one row.
//
// The three shapes below are what a second spelling LOOKS like, and each is
// refused for a reason of its own:
//
//   a ROLE on a non-input — `checkbox`, `menuitemcheckbox`, `radio` or
//   `menuitemradio` on a `<button>`, `<li>` or `<div>` — claims a control the
//   browser is not driving. Everything native radios and checkboxes give for
//   free (the group walk, the tab stop, the label's click target, the state the
//   platform reports to AT) is then the author's to rewrite, and the rewrite is
//   never complete.
//
//   `aria-checked` on anything but an input is the same claim made one
//   attribute at a time. `Switch` is the one exception and it is a different
//   control: it announces `role="switch"`, it IS the write rather than an
//   intent something later submits, and the README says which to reach for.
//
//   a BARE `<input type="checkbox">` / `type="radio"` is the right element with
//   the wrong surface — it is what `Toggle` in atoms.tsx exists to wrap, and a
//   call site that writes its own wrapper is deciding the size and the gap
//   again. `atoms.tsx` is the one file allowed to name the element.
//
// NOT in scope, because each is a different control rather than a second
// spelling of this one: `Switch` (switch.tsx), a `Select`/`MultiSelect` option
// row (a listbox option's mark is the listbox's own affordance), an
// `aria-pressed` toggle button (that is a verb, or a filter left on), and
// `SegmentedControl`.
//
// STORIES AND TESTS ARE OUT OF THE CENSUS, and that is a real hole rather than
// an oversight: `interaction.stories.tsx` renders bare native controls on
// purpose, to show what `accent-color` does to the paint the platform draws,
// and a catalogue of the platform's own controls is the one place they belong.
// A test that drives a tick drives the control the screen renders, so it needs
// no exemption either way.

import { readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  parseSource,
  scriptKindFor,
} from "../../scripts/lib/source-tree";

const frontendRoot = resolve(__dirname, "../..");
const srcDir = join(frontendRoot, "src");
// A unit's screen is shipped UI in the same bundle, so a gate stopping at
// frontend/src would hold the core to a rule the extension tier escapes.
const extensionsRoot = join(frontendRoot, "..", "extensions");

/** The roles a native input already carries, so naming one is a hand-rolled tick. */
const TICK_ROLES = new Set([
  "checkbox",
  "menuitemcheckbox",
  "radio",
  "menuitemradio",
]);

/** The input types `Toggle` wraps. */
const TICK_TYPES = new Set(["checkbox", "radio"]);

// The two files that own what the rule describes, so each is exempt from the
// arm it owns and from no other. Named by path rather than by shape: "the file
// that renders the atom" is not something a parser can recognise, and a rule
// that guessed would exempt the next hand-rolled copy too.
const ATOMS = join(srcDir, "design-system", "atoms.tsx");
const SWITCH = join(srcDir, "design-system", "switch.tsx");

// This file plants the probes below, which are deliberate examples of every
// shape. Judging them would report the gate's own evidence as a finding.
const PROBES = join(srcDir, "design-system", "tick-spelling.test.ts");

/**
 * Whether a file is a story or a test, which the census leaves out. Read off
 * the name, because that is what the shape IS — a `.stories.tsx` beside its
 * component is the co-location `fe-uat` keys on.
 */
function isStoryOrTest(path: string): boolean {
  return /\.(stories|test)\.[cm]?[jt]sx?$/.test(path);
}

/** The JSX attribute's name, for an attribute written plainly. */
function attributeName(attribute: ts.JsxAttributeLike): string | null {
  return ts.isJsxAttribute(attribute) && ts.isIdentifier(attribute.name)
    ? attribute.name.text
    : null;
}

/** A JSX attribute's value when it is a plain string, `"checkbox"` and not `{x}`. */
function stringValue(attribute: ts.JsxAttributeLike): string | null {
  if (!ts.isJsxAttribute(attribute)) return null;
  const value = attribute.initializer;
  if (value && ts.isStringLiteral(value)) return value.text;
  // `role={"checkbox"}` is the same claim wearing braces.
  if (
    value &&
    ts.isJsxExpression(value) &&
    value.expression &&
    ts.isStringLiteral(value.expression)
  ) {
    return value.expression.text;
  }
  return null;
}

/**
 * findTickSpellings returns one report line per hand-rolled tick in the source,
 * with the author's own line number.
 *
 * Read directly by the probe suite below, which is what makes the census mean
 * anything: a gate asserting a shape is ABSENT passes identically over a clean
 * tree and over a detector that has stopped detecting.
 */
function findTickSpellings(path: string, text: string): string[] {
  // A file that can spell none of the three shapes is not parsed at all: the
  // census reads the whole tree and reparses every non-TSX file as TSX.
  if (!/role|aria-checked|checkbox|radio/.test(text)) return [];
  const source = parseSource(path, text);
  const found: string[] = [];
  // Anything not already TSX is read a SECOND time as TSX and the findings
  // unioned, for native-controls.test.ts's reason: a file whose JSX the primary
  // parse cannot see yields ZERO nodes, which is a silent pass.
  if (scriptKindFor(path) !== ts.ScriptKind.TSX) {
    for (const hit of findTickSpellings(`${path}x`, text)) {
      if (!found.includes(hit)) found.push(hit);
    }
  }
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

  const visit = (node: ts.Node): void => {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const tag = ts.isIdentifier(node.tagName) ? node.tagName.text : "";
      const isInput = tag === "input";
      for (const attribute of node.attributes.properties) {
        const name = attributeName(attribute);
        if (name === "role" && !isInput) {
          const role = stringValue(attribute);
          if (role && TICK_ROLES.has(role)) {
            found.push(`${at(attribute)}: role="${role}" on <${tag}>`);
          }
        }
        if (name === "aria-checked" && !isInput && path !== SWITCH) {
          found.push(`${at(attribute)}: aria-checked on <${tag}>`);
        }
        if (name === "type" && isInput && path !== ATOMS) {
          const type = stringValue(attribute);
          if (type && TICK_TYPES.has(type)) {
            found.push(`${at(attribute)}: <input type="${type}">`);
          }
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return [...new Set(found)];
}

const ADVICE =
  "Use Checkbox or Radio from src/design-system/atoms.tsx (ChoiceList for a " +
  "whole question); see src/design-system/README.md. A setting that writes " +
  "when you flip it is a Switch, and a verb left on is a pressed Button.";

describe("one checkbox, one radio", () => {
  it("finds no hand-rolled tick under src/, outside the two atoms that own one", {
    timeout: 60_000,
  }, () => {
    const files = filesUnder(srcDir)
      .concat(extensionFrontendFiles(extensionsRoot))
      .filter((file) => file !== PROBES && !isStoryOrTest(file));

    // An empty scan means the gate is pointed at the wrong tree. A census that
    // judged nothing certifies nothing.
    expect(files.length).toBeGreaterThan(100);
    // And the screens specifically: every hand-rolled tick this gate was
    // written for lived under src/screens/, and the floor above is one the
    // design-system half could satisfy on its own.
    expect(
      files.some((file) => file.startsWith(`${srcDir}/screens/`)),
      "the census covered no screen, where every tick this gate replaced lived",
    ).toBe(true);
    // And the extension tier: a unit's screen ships in the same bundle, and
    // neither floor above can notice a walk that stops at src/.
    expect(
      files.some((file) => file.includes("/extensions/")),
      "the census reached no extension frontend layer",
    ).toBe(true);

    const violations = files.flatMap((file) =>
      findTickSpellings(file, readFileSync(file, "utf8")).map(
        (hit) => `${relative(frontendRoot, file)}:${hit}`,
      ),
    );

    expect(violations, ADVICE).toEqual([]);
  });

  it("holds the two owners to their own arm and no other", () => {
    // atoms.tsx names the element, and that is the whole of its exemption: a
    // `role="checkbox"` or an `aria-checked` on a non-input there is still a
    // finding. switch.tsx is the mirror — it carries `aria-checked` and may not
    // carry an input.
    expect(findTickSpellings(ATOMS, readFileSync(ATOMS, "utf8"))).toEqual([]);
    expect(findTickSpellings(SWITCH, readFileSync(SWITCH, "utf8"))).toEqual([]);
    expect(
      findTickSpellings(ATOMS, "const a = <button aria-checked={on} />;"),
    ).toEqual(["1: aria-checked on <button>"]);
    expect(
      findTickSpellings(SWITCH, 'const a = <input type="checkbox" />;'),
    ).toEqual(['1: <input type="checkbox">']);
  });
});

// The half that makes the census above mean anything: every case is a shape the
// rule must judge, planted so the detector cannot quietly stop detecting.
describe("the tick detector sees what it claims to", () => {
  const cases: {
    name: string;
    fires: boolean;
    src: string;
    file?: string;
    expect?: string[];
  }[] = [
    {
      name: "role=checkbox on a button",
      fires: true,
      src: 'const a = <button role="checkbox" />;',
      expect: ['1: role="checkbox" on <button>'],
    },
    {
      name: "role=menuitemcheckbox on a li",
      fires: true,
      src: 'const a = <li role="menuitemcheckbox">x</li>;',
      expect: ['1: role="menuitemcheckbox" on <li>'],
    },
    {
      name: "role=menuitemradio on a button",
      fires: true,
      src: 'const a = <button role="menuitemradio" />;',
      expect: ['1: role="menuitemradio" on <button>'],
    },
    {
      name: "role=radio on a div",
      fires: true,
      src: 'const a = <div role="radio" />;',
      expect: ['1: role="radio" on <div>'],
    },
    // The braces are the same claim: a role written `{"checkbox"}` reads
    // identically to the browser and used to read as an expression here.
    {
      name: "a role wearing braces",
      fires: true,
      src: 'const a = <button role={"checkbox"} />;',
      expect: ['1: role="checkbox" on <button>'],
    },
    {
      name: "aria-checked on a button",
      fires: true,
      src: "const a = <button aria-checked={on} />;",
      expect: ["1: aria-checked on <button>"],
    },
    {
      name: "a bare input type=checkbox",
      fires: true,
      src: 'const a = <label><input type="checkbox" /> x</label>;',
      expect: ['1: <input type="checkbox">'],
    },
    {
      name: "a bare input type=radio",
      fires: true,
      src: 'const a = <input type="radio" name="g" />;',
      expect: ['1: <input type="radio">'],
    },
    // The roles a native input already has are not a claim when the input makes
    // them — a `<input type="checkbox" role="checkbox">` is redundant, not a
    // second spelling, and lint says so where this does not.
    {
      name: "role=checkbox on the input that already is one",
      fires: false,
      src: 'const a = <input type="checkbox" role="checkbox" />;',
      file: "design-system/atoms.tsx",
    },
    {
      name: "aria-checked on an input",
      fires: false,
      src: "const a = <input type='checkbox' aria-checked={on} />;",
      file: "design-system/atoms.tsx",
    },
    // The four out-of-scope shapes, each a different control.
    {
      name: "role=switch, which is a different control",
      fires: false,
      src: 'const a = <button role="switch" aria-checked={on} />;',
      file: "design-system/switch.tsx",
    },
    {
      name: "an aria-pressed toggle button",
      fires: false,
      src: "const a = <button aria-pressed={on}>Compact</button>;",
    },
    {
      name: "a listbox option's own mark",
      fires: false,
      src: 'const a = <li role="option" aria-selected={on} />;',
    },
    {
      name: "a text input",
      fires: false,
      src: 'const a = <input type="text" />;',
    },
    // Comments are not AST nodes, so the prose that explains the rule costs
    // nothing — every file above cites the shapes it replaced.
    {
      name: "a cross-reference in a comment",
      fires: false,
      src: '// was role="menuitemradio" with aria-checked\nconst a = 1;',
    },
    {
      name: "a role read from a variable, which this cannot judge",
      fires: false,
      src: "const a = <button role={kind} />;",
    },
    // A .ts file holding JSX yields zero nodes on the primary parse, which is a
    // silent pass rather than a finding you can see.
    {
      name: "JSX in a .ts file",
      fires: true,
      src: 'export const a = <input type="radio" />;',
      file: "probe.ts",
      expect: ['1: <input type="radio">'],
    },
    {
      name: "a plain .ts file with none of it",
      fires: false,
      src: "export const a = 1;",
      file: "probe.ts",
    },
  ];

  for (const tc of cases) {
    it(`${tc.fires ? "reports" : "ignores"} ${tc.name}`, () => {
      const path = join(srcDir, tc.file ?? "probe.tsx");
      const hits = findTickSpellings(path, tc.src);
      expect(hits.length > 0, `hits: ${JSON.stringify(hits)}`).toBe(tc.fires);
      if (tc.expect) expect(hits).toEqual(tc.expect);
    });
  }

  // Every role the rule declares is one the census can actually reach. Keyed on
  // the SET, so it is four assertions today and five the moment somebody adds a
  // fifth — the pre-filter is the place a declared shape goes dark, and a file
  // skipped before it is parsed is indistinguishable from a clean one.
  it("can reach every role and every type its own rule declares", () => {
    for (const role of TICK_ROLES) {
      expect(
        findTickSpellings(
          join(srcDir, "probe.tsx"),
          `const a = <button role="${role}" />;`,
        ),
        `role="${role}" is in the rule, but the census would never parse a file that only spells it`,
      ).not.toEqual([]);
    }
    for (const type of TICK_TYPES) {
      expect(
        findTickSpellings(
          join(srcDir, "probe.tsx"),
          `const a = <input type="${type}" />;`,
        ),
        `type="${type}" is in the rule, but the census would never parse a file that only spells it`,
      ).not.toEqual([]);
    }
  });

  // Stories and tests are out of the census, and the reason is a real one:
  // interaction.stories.tsx renders bare native controls to show what
  // `accent-color` does to the paint the platform draws.
  it("leaves stories and tests out of the census", () => {
    expect(
      isStoryOrTest(join(srcDir, "design-system/interaction.stories.tsx")),
    ).toBe(true);
    expect(
      isStoryOrTest(join(srcDir, "design-system/listtable.test.tsx")),
    ).toBe(true);
    expect(isStoryOrTest(join(srcDir, "design-system/listtable.tsx"))).toBe(
      false,
    );
  });
});
