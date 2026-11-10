// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import {
  FIELD,
  findTickSpellings,
  isStoryOrTest,
  rule,
  SWITCH,
  TICK_ROLES,
  TICK_TYPES,
} from "./tick-spelling.census";

describe("one checkbox, one radio", () => {
  it("holds the two owners to their own arm and no other", () => {
    // field.tsx may name the element and nothing more. switch.tsx may carry
    // `aria-checked` and no input.
    expect(findTickSpellings(FIELD, readFileSync(FIELD, "utf8"))).toEqual([]);
    expect(findTickSpellings(SWITCH, readFileSync(SWITCH, "utf8"))).toEqual([]);
    expect(
      findTickSpellings(FIELD, "const a = <button aria-checked={on} />;"),
    ).toEqual(["1: aria-checked on <button>"]);
    expect(
      findTickSpellings(SWITCH, 'const a = <input type="checkbox" />;'),
    ).toEqual(['1: <input type="checkbox">']);
  });
});

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
    // The braces are the same claim to the browser.
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
    // A tick role on the input that already carries it is redundant, not a
    // second spelling. Lint reports it.
    {
      name: "role=checkbox on the input that already is one",
      fires: false,
      src: 'const a = <input type="checkbox" role="checkbox" />;',
      file: "design-system/field.tsx",
    },
    {
      name: "aria-checked on an input",
      fires: false,
      src: "const a = <input type='checkbox' aria-checked={on} />;",
      file: "design-system/field.tsx",
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
    // Comments are not AST nodes, so prose that cites a shape costs nothing.
    {
      name: "a cross-reference in a comment",
      fires: false,
      src: '// role="menuitemradio" with aria-checked\nconst a = 1;',
    },
    {
      name: "a role read from a variable, which this cannot judge",
      fires: false,
      src: "const a = <button role={kind} />;",
    },
    // A .ts file holding JSX yields no nodes on the primary parse.
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

  // Keyed on the sets, so a role or type added to the rule is checked here too.
  // The prefilter is where a declared shape would go dark.
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

describe("the tick-spelling rule", () => {
  it("reports a planted tick through its own corpus and visit", () => {
    const text = 'const a = <li role="menuitemradio">x</li>;';
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "planted.tsx"), area: "src", text },
        {
          path: join(srcDir, "features", "planted.stories.tsx"),
          area: "src",
          text,
        },
      ]),
    ).toEqual(['src/features/planted.tsx:1: role="menuitemradio" on <li>']);
  });
});
