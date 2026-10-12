// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The split gates' shared walk, held against a fixture: a walk that saw no
// edge, or a corpus that dropped a shipped file, would let every gate built on
// it report PASS over a split that had collapsed.

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  edgesOf,
  importPathTo,
  productionModulesUnder,
  testRunnerReach,
} from "./import-reach";

describe("the import walk the split gates share", () => {
  let dir = "";
  const at = (name: string) => join(dir, name);
  const write = (name: string, text = "") => {
    mkdirSync(dirname(at(name)), { recursive: true });
    writeFileSync(at(name), text);
  };
  const named = (paths: readonly string[] | null) =>
    paths?.map((path) => path.slice(dir.length + 1)) ?? null;

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "import-reach-"));
  });
  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it("counts every shipped module, at any depth, and no test, story or testkit", () => {
    for (const name of [
      "a.ts",
      "b.tsx",
      "nested/c.tsx",
      "a.test.ts",
      "b.test.tsx",
      "b.stories.tsx",
      "d.testkit.ts",
      "e.testkit.tsx",
      "f.css",
    ]) {
      write(name);
    }

    expect(named(productionModulesUnder(dir))?.sort()).toEqual([
      "a.ts",
      "b.tsx",
      "nested/c.tsx",
    ]);
  });

  it("finds a target several hops in, through a type-only edge, by the shortest path", () => {
    write("entry.ts", 'import { a } from "./mid";\nimport "./long";\n');
    write("mid.ts", 'import type { B } from "./heavy";\nexport const a = 1;\n');
    write("long.ts", 'import "./longer";\n');
    write("longer.ts", 'import "./heavy";\n');
    write("heavy.tsx", "export type B = number;\n");

    expect(
      named(importPathTo(at("entry.ts"), new Set([at("heavy.tsx")]))),
    ).toEqual(["entry.ts", "mid.ts", "heavy.tsx"]);
  });

  it("answers the entry alone when the entry is itself a target", () => {
    write("heavy.tsx", 'import "./other";\n');
    write("other.ts", 'import "./heavy";\n');

    expect(
      named(importPathTo(at("heavy.tsx"), new Set([at("heavy.tsx")]))),
    ).toEqual(["heavy.tsx"]);
  });

  it("answers null when no target is reachable", () => {
    write("entry.ts", 'import "./mid";\n');
    write("mid.ts", 'import "./entry";\n');
    write("heavy.tsx", "");

    expect(importPathTo(at("entry.ts"), new Set([at("heavy.tsx")]))).toBeNull();
  });

  it("scans every edge the parser finds, in every import form", () => {
    write(
      "forms.tsx",
      [
        'import a from "./a";',
        'import * as b from "./b";',
        'import { type C, c } from "./c";',
        'import type { D } from "./d";',
        'import "./e";',
        'export { f } from "./f";',
        'export * from "./g";',
        'export type { H } from "./h";',
        'import {\n  i,\n} from "./i";',
        'const j = () => import(/* lazy */ "./j");',
        "const l = () => import(`./l`);",
        'export const k = <p>{"./k"}</p>;',
      ].join("\n"),
    );
    const modules = ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "l"];
    for (const name of [...modules, "k"]) {
      write(`${name}.ts`);
    }
    const expected = modules.map((name) => `${name}.ts`);

    expect(named(edgesOf(at("forms.tsx"), "all"))?.sort()).toEqual(expected);
    expect(named(edgesOf(at("forms.tsx"), "scanned"))?.sort()).toEqual(
      expected,
    );
  });

  it("walks value edges alone when asked, keeping a separate cache per mode", () => {
    write("kit.stories.tsx", 'import type { M } from "./kit.testkit";\n');
    write("kit.testkit.ts", "export type M = number;\n");
    const kit = new Set([at("kit.testkit.ts")]);

    expect(importPathTo(at("kit.stories.tsx"), kit, "values")).toBeNull();
    expect(named(importPathTo(at("kit.stories.tsx"), kit))).toEqual([
      "kit.stories.tsx",
      "kit.testkit.ts",
    ]);
  });

  it("finds every value path to vitest from stories, docs pages and preview", () => {
    const vi = 'import { vi } from "vitest";\nexport const body = vi.fn();\n';
    write(
      "src/card.stories.tsx",
      'import { body } from "./plain";\nimport "./typed.fixtures";\n',
    );
    write("src/plain.ts", 'export { body } from "./card.fixtures";\n');
    write("src/card.fixtures.ts", vi);
    write(
      "src/typed.fixtures.ts",
      'import type { Mock } from "vitest";\nexport type M = Mock;\n',
    );
    write("src/typed.stories.tsx", 'import "./typed.fixtures";\n');
    write("src/kit.stories.tsx", 'import type { M } from "./kit.testkit";\n');
    write("src/kit.testkit.ts", `${vi}export type M = number;\n`);
    write("src/spy.stories.tsx", 'import { fn } from "@vitest/spy";\n');
    write(
      "src/dom.stories.tsx",
      'import "@testing-library/jest-dom/vitest";\n',
    );
    write("src/far.stories.tsx", 'import { body } from "../outside";\n');
    write("outside.ts", vi);
    write(".storybook/preview.tsx", vi);
    write(
      "src/intro.mdx",
      'import { Meta } from "@storybook/addon-docs/blocks";\n' +
        'import { body } from "./card.fixtures";\n\n# Intro\n',
    );

    write(
      "src/notes.mdx",
      'See import("./kit.testkit") in prose.\n\n' +
        '```ts\nimport { vi } from "vitest";\n```\n',
    );
    write(
      "src/reexport.mdx",
      '# Re-export\n\nexport { body } from "./card.fixtures";\n',
    );
    write("src/tilde.mdx", '~~~ts\nimport { vi } from "vitest";\n~~~\n');
    write(
      "src/nested.mdx",
      '````md\n```\nimport { vi } from "vitest";\n```\n````\n',
    );

    const entries = [
      "src/card.stories.tsx",
      "src/typed.stories.tsx",
      "src/kit.stories.tsx",
      "src/spy.stories.tsx",
      "src/dom.stories.tsx",
      "src/far.stories.tsx",
      ".storybook/preview.tsx",
      "src/intro.mdx",
      "src/notes.mdx",
      "src/reexport.mdx",
      "src/tilde.mdx",
      "src/nested.mdx",
    ];

    expect(testRunnerReach(entries.map(at)).map(named)).toEqual([
      ["src/card.stories.tsx", "src/plain.ts", "src/card.fixtures.ts"],
      ["src/spy.stories.tsx"],
      ["src/dom.stories.tsx"],
      ["src/far.stories.tsx", "outside.ts"],
      [".storybook/preview.tsx"],
      ["src/intro.mdx", "src/card.fixtures.ts"],
      ["src/reexport.mdx", "src/card.fixtures.ts"],
    ]);
  });
});
