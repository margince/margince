// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The split gates' shared walk, held against a fixture: a walk that saw no
// edge, or a corpus that dropped a shipped file, would let every gate built on
// it report PASS over a split that had collapsed.

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { importPathTo, productionModulesUnder } from "./import-reach";

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
});
