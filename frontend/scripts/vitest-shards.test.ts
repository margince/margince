// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { afterAll, describe, expect, it } from "vitest";
import { filesMatching } from "./lib/source-tree";
import { testCosts } from "./lib/test-cost";
import { compilerTests, partition } from "./vitest-shards";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

// Any corpus proves the split; the tree's own test files make it a real one.
// check-shard-union.sh proves the same of each CI run against vitest's discovery.
const suite = testCosts([
  ...filesMatching(join(frontendRoot, "src"), /\.test\.tsx?$/),
  ...filesMatching(join(frontendRoot, "scripts"), /\.test\.tsx?$/),
]);
const costs = new Map([...suite].map(([file, { cost }]) => [file, cost]));

describe("the cost-weighted shard split", () => {
  it.each([1, 2, 3, 4, 6, 8])(
    "puts every file in exactly one of %i slices",
    (count) => {
      const slices = partition(costs, count);
      expect(slices).toHaveLength(count);
      const placed = slices.flat();
      expect(placed).toHaveLength(costs.size);
      expect(new Set(placed)).toEqual(new Set(costs.keys()));
    },
  );

  it("splits the same way whatever order the files arrive in", () => {
    const reversed = new Map([...costs].reverse());
    expect(partition(reversed, 4)).toEqual(partition(costs, 4));
  });

  it("gives each of the heaviest files a slice of its own", () => {
    const heaviest = [...costs].sort((a, b) => b[1] - a[1]).slice(0, 4);
    const sliceOf = (file: string) =>
      partition(costs, 4).findIndex((slice) => slice.includes(file));
    expect(new Set(heaviest.map(([file]) => sliceOf(file))).size).toBe(4);
  });

  it("refuses a slice count that is not a positive integer", () => {
    expect(() => partition(costs, 0)).toThrow(/positive integer/);
  });
});

describe("the cost a test file is expected to have", () => {
  const dir = mkdtempSync(join(tmpdir(), "test-cost-"));
  afterAll(() => rmSync(dir, { recursive: true, force: true }));
  const plant = (name: string, source: string) => {
    const path = join(dir, name);
    writeFileSync(path, source);
    return path;
  };
  plant(
    "compiler.ts",
    'import ts from "typescript";\nexport const k = ts.SyntaxKind;\n',
  );
  plant(
    "typesonly.ts",
    'import type ts from "typescript";\nexport type K = ts.SyntaxKind;\n',
  );
  plant("cycle-a.ts", 'import { b } from "./cycle-b";\nexport { b };\n');
  plant(
    "cycle-b.ts",
    'import "./cycle-a";\nimport { k } from "./compiler";\nexport const b = k;\n',
  );
  const kindOf = (name: string, source: string) =>
    testCosts([plant(name, source)]).get(join(dir, name))?.kind;

  it("reads a file that never loads the compiler as a render test", () => {
    expect(kindOf("plain.test.ts", 'import { expect } from "vitest";\n')).toBe(
      "render",
    );
  });

  it("does not count a type-only import, which loads nothing", () => {
    expect(
      kindOf("types.test.ts", 'import type { K } from "./typesonly";\n'),
    ).toBe("render");
  });

  it("follows the import graph through a cycle to the compiler", () => {
    expect(kindOf("scan.test.ts", 'import { b } from "./cycle-a";\n')).toBe(
      "parse",
    );
  });

  it("tells a file that builds a program from one that only parses", () => {
    const source = 'import { k } from "./compiler";\nprogramOver([k], "");\n';
    expect(kindOf("program.test.ts", source)).toBe("program");
  });

  it("reads a test's sweep module as the test's own code", () => {
    plant(
      "census.sweep.ts",
      'import { k } from "./compiler";\nprogramOver([k], "");\n',
    );
    const source = 'import type * as S from "./census.sweep";\n';
    expect(kindOf("swept.test.ts", source)).toBe("program");
  });

  it("finds all three kinds in the real suite", () => {
    const kinds = new Set([...suite.values()].map(({ kind }) => kind));
    expect(kinds).toEqual(new Set(["render", "parse", "program"]));
  });
});

describe("the unisolated project of compiler-loading files", () => {
  it("holds every file the cost model reads as parse or program", async () => {
    const scans = new Set(await compilerTests(frontendRoot));
    const expected = [...suite]
      .filter(([, { kind }]) => kind !== "render")
      .map(([file]) => relative(frontendRoot, file));
    expect(expected.length).toBeGreaterThan(0);
    expect(expected.filter((file) => !scans.has(file))).toEqual([]);
  });
});
