// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir, treeOf } from "../tree";
import { englishGate } from "./copy-style-vi.census";
import { rule } from "./mcp-view-copy.census";

const gate = readFileSync(englishGate, "utf8");
const view = (path: string) => join(srcDir, "mcp-apps", path);

const planted = (files: Record<string, string>, gateText = gate) => [
  { path: englishGate, area: "src" as Area, text: gateText },
  ...Object.entries(files).map(([path, text]) => ({
    path: view(path),
    area: "src" as Area,
    text,
  })),
];

const findingsOver = (files: Record<string, string>) =>
  plantedFindings(rule, planted(files));

describe("the mcp-view-copy rule, over planted views", () => {
  it("passes clean copy", () => {
    expect(
      findingsOver({
        "card/main.ts": `const t = ["The forecast’s inputs", \`Due \${d}\`];\n`,
        "card/index.html": '<title>The card</title><img alt="Logo" />',
      }),
    ).toEqual([]);
  });

  it.each([
    ["uses no em dash or en dash", "Saved — done"],
    ["spells apostrophes and quotes curly", "The forecast's inputs"],
    ["uses no contraction", "It can’t run"],
    ["never speaks as we", "Who do we know"],
    ["says I only inside the onboarding conversation", "What should I do"],
    ["uses no exclamation mark", "Done!"],
    ["spells an ellipsis as the one character …", "Saving..."],
    ["has no double space", "Saved  now"],
    ["uses no Latin abbreviation", "Deals, e.g. renewals"],
    ["never says please", "Please retry"],
    ["spells American English", "Cancelled"],
    ["writes and, never an ampersand", "Won & lost"],
    ["uses no retired word", "Open promises"],
  ])("runs the gate's check %s", (check, value) => {
    expect(
      findingsOver({ "card/main.ts": `const t = ${JSON.stringify(value)};\n` }),
    ).toEqual([
      `${check}: src/mcp-apps/card/main.ts:1:11 ${JSON.stringify(value)}`,
    ]);
  });

  it("reads a placeholder in a template as {}", () => {
    expect(
      findingsOver({
        "card/main.ts": `const t = \`Catch me up on "\${name}"\`;\n`,
      }),
    ).toEqual([
      'spells apostrophes and quotes curly: src/mcp-apps/card/main.ts:1:11 "Catch me up on \\"{}\\""',
    ]);
  });

  it("reads JSX text, JSX attributes and every page label", () => {
    expect(
      findingsOver({
        "card/panel.tsx":
          "const p = <p aria-label=\"Don't\">  It's saved  </p>;\n",
        "card/index.html":
          '<title>The card\'s page</title>\n<img alt="A\'s" /><input placeholder="B\'s" aria-label="C\'s" title="D\'s" />',
      }),
    ).toEqual([
      'spells apostrophes and quotes curly: src/mcp-apps/card/panel.tsx:1:25 "Don\'t"',
      'spells apostrophes and quotes curly: src/mcp-apps/card/panel.tsx:1:35 "It\'s saved"',
      'spells apostrophes and quotes curly: src/mcp-apps/card/index.html:<title> "The card\'s page"',
      'spells apostrophes and quotes curly: src/mcp-apps/card/index.html:2:alt "A\'s"',
      'spells apostrophes and quotes curly: src/mcp-apps/card/index.html:2:placeholder "B\'s"',
      'spells apostrophes and quotes curly: src/mcp-apps/card/index.html:2:aria-label "C\'s"',
      'spells apostrophes and quotes curly: src/mcp-apps/card/index.html:2:title "D\'s"',
      'uses no contraction: src/mcp-apps/card/panel.tsx:1:25 "Don\'t"',
      'uses no contraction: src/mcp-apps/card/panel.tsx:1:35 "It\'s saved"',
    ]);
  });

  it("skips tests, stories, testkits, fixtures and the Storybook hosts", () => {
    expect(
      findingsOver({
        "card/main.ts": 'const t = "Saved";\n',
        "card/index.html": "<title>The card</title>",
        "card/main.test.ts": 'const t = "we";\n',
        "card/card.stories.tsx": 'const t = "we";\n',
        "card/card.testkit.ts": 'const t = "we";\n',
        "card/fixture.ts": 'const t = "we";\n',
        "story-hosts.tsx": 'const t = "we";\n',
      }),
    ).toEqual([]);
  });

  it("fails on a gate check whose pattern it cannot read", () => {
    const unreadable = gate.replace(
      "expect(offenders(/!/)).toEqual([]);",
      "expect(offenders(bang())).toEqual([]);",
    );
    expect(unreadable).not.toBe(gate);
    expect(() =>
      plantedFindings(
        rule,
        planted({ "card/main.ts": 'const t = "Saved";\n' }, unreadable),
      ),
    ).toThrow(
      /"uses no exclamation mark" checks a pattern this rule cannot read/,
    );
  });

  it("states a floor when no view page or module is read", () => {
    const tree = treeOf(planted({ "card/main.ts": 'const t = "Saved";\n' }));
    expect(rule.floors?.(rule.corpus(tree), tree)).toEqual([
      "reads no view page under src/mcp-apps",
    ]);
  });
});
