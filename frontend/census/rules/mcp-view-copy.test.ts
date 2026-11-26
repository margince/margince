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

const findingsOver = (files: Record<string, string>, gateText = gate) =>
  plantedFindings(rule, planted(files, gateText));

const module = (text: string) => findingsOver({ "card/main.ts": text });

const DESCRIBE = 'describe("en copy style", () => {';
const IMPORTS = 'import { en } from "./en";';

/** The gate with `test` added as its first test and `topLevel` after its
 *  imports. */
function gateWith(test: string, topLevel = ""): string {
  const edited = gate
    .replace(DESCRIBE, `${DESCRIBE}\n${test}\n`)
    .replace(IMPORTS, `${IMPORTS}\n${topLevel}\n`);
  expect(edited).not.toBe(gate);
  return edited;
}

const quoted = (check: string, at: string, value: string) =>
  `${check}: ${at} ${JSON.stringify(value)}`;
const CURLY = "spells apostrophes and quotes curly";

describe("the mcp-view-copy rule, over planted views", () => {
  it("passes clean copy", () => {
    expect(
      findingsOver({
        "card/main.ts": `const t = ["The forecast’s inputs", \`Due \${d}\`];\n`,
        "card/index.html": '<title>The card</title><img alt="Logo" />',
        "view.css": '.a::after { content: "→" / ""; }',
      }),
    ).toEqual([]);
  });

  it.each([
    ["uses no em dash or en dash", "Saved — done"],
    [CURLY, "The forecast's inputs"],
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
    expect(module(`const t = ${JSON.stringify(value)};\n`)).toEqual([
      quoted(check, "src/mcp-apps/card/main.ts:1:11", value),
    ]);
  });

  it("reads a template placeholder and any part of a grouped + chain as {}", () => {
    expect(
      module(
        `const a = \`Catch me up on "\${name}"\`;\nconst b = "It" + ("'s due " + day);\n`,
      ),
    ).toEqual([
      quoted(CURLY, "src/mcp-apps/card/main.ts:1:11", 'Catch me up on "{}"'),
      quoted(CURLY, "src/mcp-apps/card/main.ts:2:11", "It's due {}"),
      quoted(
        "uses no contraction",
        "src/mcp-apps/card/main.ts:2:11",
        "It's due {}",
      ),
    ]);
  });

  it("keeps a wire value as the wire spells it, and still reads copy beside it", () => {
    expect(
      module(
        [
          'type S = "cancelled" | "open";',
          'const done = status === "cancelled" || status != "cancelled";',
          'switch (status) { case "cancelled": break; }',
          'const t = "Cancelled order";',
        ].join("\n"),
      ),
    ).toEqual([
      quoted(
        "spells American English",
        "src/mcp-apps/card/main.ts:4:11",
        "Cancelled order",
      ),
    ]);
  });

  it("reads JSX text and attributes with their entities decoded", () => {
    expect(
      findingsOver({
        "card/panel.tsx":
          'const p = <p aria-label="Don&apos;t">\n  Won &amp;\n  lost\n</p>;\n',
      }),
    ).toEqual([
      quoted(CURLY, "src/mcp-apps/card/panel.tsx:1:25", "Don't"),
      quoted(
        "uses no contraction",
        "src/mcp-apps/card/panel.tsx:1:25",
        "Don't",
      ),
      quoted(
        "writes and, never an ampersand",
        "src/mcp-apps/card/panel.tsx:2:3",
        "Won & lost",
      ),
    ]);
  });

  it("reads a page's text and labels, and none of its script or style", () => {
    const page = [
      '<html><head><title lang="en">',
      "  The card&apos;s",
      "  page</title>",
      `<meta name="description" content='A "b"'>`,
      "<script>const we = 1;</script><style>p { color: red }</style>",
      "</head><body><p>We know it</p>",
      '<img alt=\'A&apos;s\' /><input placeholder="Find…" aria-describedby="Us"',
      ' title="Don’t',
      '  stop" /></body></html>',
    ].join("\n");
    expect(findingsOver({ "card/index.html": page })).toEqual([
      quoted(CURLY, "src/mcp-apps/card/index.html <title>", "The card's page"),
      quoted(CURLY, "src/mcp-apps/card/index.html <meta content>", 'A "b"'),
      quoted(CURLY, "src/mcp-apps/card/index.html <img alt>", "A's"),
      quoted(
        "uses no contraction",
        "src/mcp-apps/card/index.html <input title>",
        "Don’t stop",
      ),
      quoted(
        "never speaks as we",
        "src/mcp-apps/card/index.html <p>",
        "We know it",
      ),
      quoted(
        "never speaks as we",
        "src/mcp-apps/card/index.html <input aria-describedby>",
        "Us",
      ),
    ]);
  });

  it("reads the content strings of a view sheet", () => {
    expect(
      findingsOver({
        "view.css": `.row::before { content: "→" / "it's done"; }`,
        "story-hosts.css": `.x::before { content: "we"; }`,
      }),
    ).toEqual([
      quoted(CURLY, "src/mcp-apps/view.css .row::before", "it's done"),
      quoted(
        "uses no contraction",
        "src/mcp-apps/view.css .row::before",
        "it's done",
      ),
    ]);
  });

  it("names a JSON import whose copy it does not read", () => {
    expect(
      module(
        'import tools from "../actions.json";\nimport copy from "./copy.json";\nimport actions from "./actions.json";\n',
      ),
    ).toEqual([
      "reads JSON this rule cannot: src/mcp-apps/card/main.ts imports ./copy.json",
      "reads JSON this rule cannot: src/mcp-apps/card/main.ts imports ./actions.json",
    ]);
  });

  it("skips tests, stories, testkits, fixtures and the Storybook hosts", () => {
    expect(
      findingsOver({
        "card/main.ts": 'const t = "Saved";\n',
        "card/main.test.ts": 'const t = "we";\n',
        "card/card.stories.tsx": 'const t = "we";\n',
        "card/card.testkit.ts": 'const t = "we";\n',
        "card/fixture.ts": 'const t = "we";\n',
        "story-hosts.tsx": 'const t = "we";\n',
      }),
    ).toEqual([]);
  });

  it("tests a stateful pattern once per value", () => {
    const sticky = gateWith(
      'it("uses no tilde", () => { expect(offenders(/~/g)).toEqual([]); });',
    );
    expect(
      findingsOver(
        { "card/main.ts": 'const a = "a~";\nconst b = "b~";\n' },
        sticky,
      ),
    ).toEqual([
      quoted("uses no tilde", "src/mcp-apps/card/main.ts:1:11", "a~"),
      quoted("uses no tilde", "src/mcp-apps/card/main.ts:2:11", "b~"),
    ]);
  });

  it("resolves a name in its own check, then at the gate's top level", () => {
    const shadowed = gateWith(
      [
        'it("uses no zed", () => { const TILDE = /z/; expect(offenders(TILDE)).toEqual([]); });',
        'it("uses no tilde", () => { expect(offenders(TILDE)).toEqual([]); });',
      ].join("\n"),
      "const TILDE = /~/;",
    );
    expect(
      findingsOver({ "card/main.ts": 'const t = "a~";\n' }, shadowed),
    ).toEqual([
      quoted("uses no tilde", "src/mcp-apps/card/main.ts:1:11", "a~"),
    ]);
  });

  it.each([
    [
      "an it.each check",
      gateWith(
        'it.each([[/~/]])("uses no tilde", (p) => { expect(offenders(p)).toEqual([]); });',
      ),
      /"uses no tilde" reads p, which it does not declare/,
    ],
    [
      "a pattern built by a call",
      gateWith(
        'it("uses no tilde", () => { expect(offenders(tilde())).toEqual([]); });',
        "function tilde(): RegExp { return /~/; }",
      ),
      /"uses no tilde" checks a pattern this rule cannot read/,
    ],
    [
      "a test() check without offenders",
      gateWith(
        'test(`uses no tilde`, () => { expect(["a"].filter((v) => /~/.test(v))).toEqual([]); });',
      ),
      /"uses no tilde" gives no offenders\(…\) check/,
    ],
    [
      "a wrapper of offenders",
      gateWith("", "const check = (p: RegExp) => offenders(p);"),
      /reaches offenders outside a check this rule reads, at line \d+/,
    ],
    [
      "a free variable in a text function",
      gateWith(
        'it("uses no tilde", () => { expect(offenders(/~/, { text: marked })).toEqual([]); });',
        "function marked(value: string): string { return value + MARK; }",
      ),
      /"uses no tilde" reads MARK through marked, which this rule cannot bind/,
    ],
    [
      "options that are not an object literal",
      gateWith(
        'it("uses no tilde", () => { expect(offenders(/~/, opts)).toEqual([]); });',
      ),
      /"uses no tilde" passes options this rule cannot read/,
    ],
    [
      "a stale entry among the tests that check no copy",
      `${gate.slice(0, gate.indexOf('  it("retires only words'))}});\n`,
      /has no test retires only words/,
    ],
  ])("fails on %s", (_, gateText, error) => {
    expect(gateText).not.toBe(gate);
    expect(() =>
      findingsOver({ "card/main.ts": 'const t = "Saved";\n' }, gateText),
    ).toThrow(error);
  });

  it("states a floor for each kind of view file it reads none of", () => {
    const tree = treeOf(planted({ "card/main.ts": 'const t = "Saved";\n' }));
    expect(rule.floors?.(rule.corpus(tree), tree)).toEqual([
      "reads no view page under src/mcp-apps",
      "reads no view sheet under src/mcp-apps",
    ]);
  });
});
