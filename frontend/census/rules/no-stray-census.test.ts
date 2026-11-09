// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import {
  computedImports,
  ownWalks,
  rule,
  strayImports,
} from "./no-stray-census.census";

const planted = join(srcDir, "features", "planted.test.ts");
const found = (code: string) =>
  strayImports(planted, parseSource(planted, code));

describe("a test file outside census/ that walks the tree", () => {
  it("is found under an alias", () => {
    expect(
      found(
        'import { filesUnder as walk } from "../../scripts/lib/source-tree";\nwalk(".");',
      ),
    ).toEqual(["imports filesUnder from scripts/lib/source-tree.ts"]);
  });

  // A computed name can be a walker's, and only running the test says which.
  it("is found for a dynamic import whose name is computed", () => {
    const computed = (code: string) =>
      computedImports(parseSource(planted, code));
    expect(computed("const m = await import(name);")).toEqual([
      "imports a module whose name is computed: import(name)",
    ]);
    expect(computed("const m = await import(`./$" + "{name}`);")).toHaveLength(
      1,
    );
    expect(computed("const m = await import(`./fixed`);")).toEqual([]);
  });

  it("is found through a namespace import and a dynamic import", () => {
    expect(
      found('import * as tree from "../../scripts/lib/source-tree.ts";'),
    ).toEqual(["imports all of scripts/lib/source-tree.ts"]);
    expect(found('const m = await import("../testing/screenfiles");')).toEqual([
      "imports all of src/testing/screenfiles.ts",
    ]);
  });

  it("is found for a corpus module as well as the walk itself", () => {
    expect(
      found('import { screenFiles } from "../testing/screenfiles";'),
    ).toEqual(["imports screenFiles from src/testing/screenfiles.ts"]);
  });

  it("is not a test that only parses, or imports a walker's type", () => {
    expect(
      found(
        'import { parseSource } from "../../scripts/lib/source-tree";\nimport type { filesUnder } from "../../scripts/lib/source-tree";\nimport { type filesMatching } from "../../scripts/lib/source-tree";',
      ),
    ).toEqual([]);
  });
});

describe("a test file that writes its own walk", () => {
  const walks = (code: string) =>
    ownWalks(
      parseSource(
        planted,
        `import { readdirSync as ls } from "node:fs";\n${code}`,
      ),
    );

  it("is found when a function lists a directory and calls itself", () => {
    expect(
      walks(
        "function sources(dir) { return ls(dir).flatMap((e) => sources(e)); }",
      ),
    ).toEqual(["walks the tree itself in sources()"]);
  });

  it("is found when readdirSync is asked to recurse", () => {
    expect(walks('ls("src", { recursive: true });')).toEqual([
      "walks the tree itself with a recursive readdirSync",
    ]);
  });

  it("is found when import.meta.glob reaches under **", () => {
    expect(walks('import.meta.glob(["./a/*.ts", "../**/*.tsx"]);')).toEqual([
      "walks the tree itself with import.meta.glob over **",
    ]);
  });

  it("is not one directory listed once", () => {
    expect(
      walks('const pages = ls("docs"); import.meta.glob("./*.json");'),
    ).toEqual([]);
  });
});

describe("the no-stray-census rule", () => {
  it("fails a stray through its own corpus and across", () => {
    const stray = join(srcDir, "features", "planted-stray.test.ts");
    expect(
      plantedFindings(rule, [
        {
          path: stray,
          area: "src",
          text: 'import { filesUnder } from "../../scripts/lib/source-tree";\n',
        },
      ]),
    ).toContain(
      "src/features/planted-stray.test.ts: imports filesUnder from scripts/lib/source-tree.ts",
    );
  });
});

describe("a helper module that walks the tree for a test", () => {
  const helper = join(srcDir, "features", "plantedhelper.ts");
  const test = join(srcDir, "features", "planted-helper.test.ts");
  const run = (helperText: string, testText: string) =>
    plantedFindings(rule, [
      { path: helper, area: "src", text: helperText },
      { path: test, area: "src", text: testText },
    ]).filter((finding) => finding.startsWith("src/features/planted-helper"));
  const walker =
    'import { filesUnder } from "../../scripts/lib/source-tree";\n';

  it("is found when the helper walks as it loads", () => {
    expect(
      run(
        `${walker}export const FILES = filesUnder(".");\n`,
        'import { FILES } from "./plantedhelper";\n',
      ),
    ).toEqual([
      "src/features/planted-helper.test.ts: walks the tree through src/features/plantedhelper.ts, which calls filesUnder from scripts/lib/source-tree.ts when it loads",
    ]);
  });

  it("is found when the test calls a helper function that walks", () => {
    expect(
      run(
        `${walker}export function files() {\n  return filesUnder(".");\n}\n`,
        'import { files } from "./plantedhelper";\nfiles();\n',
      ),
    ).toEqual([
      "src/features/planted-helper.test.ts: walks the tree through files() in src/features/plantedhelper.ts, which calls filesUnder from scripts/lib/source-tree.ts",
    ]);
  });

  it("is not a helper whose walk the test never reaches", () => {
    expect(
      run(
        `${walker}export function files() {\n  return filesUnder(".");\n}\nexport const pure = (n: number) => n + 1;\n`,
        'import { pure } from "./plantedhelper";\npure(1);\n',
      ),
    ).toEqual([]);
  });

  const helper2 = join(srcDir, "features", "plantedhelper2.ts");
  const chain = (first: string, second: string, testText: string) =>
    plantedFindings(rule, [
      { path: helper, area: "src", text: first },
      { path: helper2, area: "src", text: second },
      { path: test, area: "src", text: testText },
    ]).filter((finding) => finding.startsWith("src/features/planted-helper"));

  it("is found when a helper walks through a second helper", () => {
    expect(
      chain(
        'import { files } from "./plantedhelper2";\nexport const listed = () => files();\n',
        `${walker}export function files() {\n  return filesUnder(".");\n}\n`,
        'import { listed } from "./plantedhelper";\nlisted();\n',
      ),
    ).toHaveLength(1);
    expect(
      chain(
        'import { FILES } from "./plantedhelper2";\nexport const count = FILES.length;\n',
        `${walker}export const FILES = filesUnder(".");\n`,
        'import { count } from "./plantedhelper";\n',
      ),
    ).toHaveLength(1);
  });

  it("is not a helper that walks only when run as a script", () => {
    expect(
      run(
        `${walker}export const pure = (n: number) => n + 1;\nif (process.argv[1] === "x") {\n  filesUnder(".");\n}\n`,
        'import { pure } from "./plantedhelper";\npure(1);\n',
      ),
    ).toEqual([]);
  });

  it("is found when a helper writes its own walk", () => {
    expect(
      run(
        'import { readdirSync } from "node:fs";\nexport function deep(dir: string): string[] {\n  return readdirSync(dir).flatMap((name) => deep(name));\n}\n',
        'import { deep } from "./plantedhelper";\ndeep(".");\n',
      ),
    ).toHaveLength(1);
    expect(
      run(
        'export const all = import.meta.glob("../**/*.ts");\n',
        'import { all } from "./plantedhelper";\n',
      ),
    ).toHaveLength(1);
  });

  it("is found when a helper imports a module by a computed name", () => {
    expect(
      run(
        "export const load = (name: string) => import(name);\n",
        'import { load } from "./plantedhelper";\n',
      ),
    ).toHaveLength(1);
  });

  it("is found when a test imports a walking helper with a literal import()", () => {
    expect(
      run(
        `${walker}export function files() {\n  return filesUnder(".");\n}\n`,
        'const helper = await import("./plantedhelper");\nhelper.files();\n',
      ),
    ).toHaveLength(1);
  });
});

describe("a walker's own test", () => {
  it("fails when it walks the real tree rather than a planted one", () => {
    const own = join(srcDir, "..", "scripts", "lib", "story-files.test.ts");
    expect(
      plantedFindings(rule, [
        {
          path: own,
          area: "scripts",
          text: 'import { filesUnder } from "./source-tree";\nfilesUnder(new URL("..", import.meta.url).pathname);\n',
        },
      ]),
    ).toEqual([
      "scripts/lib/story-files.test.ts is listed as a walker's own test but finds the real tree through import.meta.url",
    ]);
  });
});
