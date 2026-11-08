// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { srcDir } from "../tree";
import { ownWalks, readWaivers, strayImports } from "./no-stray-census.census";

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

  it("reads the waiver list as paths, past comments and blank lines", () => {
    expect([
      ...readWaivers("# why\n\nsrc/a.test.ts\n  src/b.test.ts  \n"),
    ]).toEqual(["src/a.test.ts", "src/b.test.ts"]);
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
