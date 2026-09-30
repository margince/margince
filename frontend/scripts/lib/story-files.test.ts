// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Against a fixture tree: the real one cannot show the census reading a new
// kind of story file, or refusing a glob it cannot read.

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { storyCensus, storySuffixes } from "./story-files";

const main = (globs: string) =>
  `const config = { stories: [${globs}] } satisfies Config;\nexport default config;`;

describe("storySuffixes reads the endings off the stories globs", () => {
  const probe = "main.ts";

  it("reads alternatives and bare endings alike", () => {
    expect(
      storySuffixes(probe, main('"../src/**/*.stories.@(ts|tsx)"')),
    ).toEqual([".stories.ts", ".stories.tsx"]);
    expect(
      storySuffixes(
        probe,
        main('"../src/**/*.mdx", "../src/**/*.stories.@(ts|tsx)"'),
      ),
    ).toEqual([".mdx", ".stories.ts", ".stories.tsx"]);
    expect(storySuffixes(probe, main('"../src/**/*.stories.tsx"'))).toEqual([
      ".stories.tsx",
    ]);
    expect(
      storySuffixes(
        probe,
        'export default <C>{ stories: ["../src/**/*.stories.@(tsx)"] };',
      ),
    ).toEqual([".stories.tsx"]);
  });

  it("reports no endings rather than guessing any", () => {
    for (const globs of [
      "",
      "GLOBS",
      '"../src/**/*.stories.@(ts|tsx)", "../extensions/**/*.stories.tsx"',
      '"../src/**/*"',
      '"../src/screens/*.stories.tsx"',
    ]) {
      expect(storySuffixes(probe, main(globs))).toBe(null);
    }
    expect(
      storySuffixes(
        probe,
        'export const stories = ["../src/**/*.stories.@(tsx)"];',
      ),
    ).toBe(null);
  });
});

describe("storyCensus walks every file Storybook loads", () => {
  let root: string;

  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), "story-census-"));
    mkdirSync(join(root, ".storybook"));
    mkdirSync(join(root, "src", "screens"), { recursive: true });
    for (const file of [
      "src/introduction.mdx",
      "src/screens/brief.stories.tsx",
      "src/screens/brief.tsx",
      "src/screens/brief.stories.ts.snap",
    ]) {
      writeFileSync(join(root, file), "");
    }
  });

  afterEach(() => {
    rmSync(root, { recursive: true, force: true });
  });

  it("reads the files every glob names, and no others", () => {
    writeFileSync(
      join(root, ".storybook", "main.ts"),
      main('"../src/**/*.mdx", "../src/**/*.stories.@(ts|tsx)"'),
    );
    const census = storyCensus(root);
    expect(census.files.map((file) => relative(root, file)).sort()).toEqual([
      "src/introduction.mdx",
      "src/screens/brief.stories.tsx",
    ]);
  });

  it("reads nothing, and says so, when a glob cannot be read", () => {
    writeFileSync(join(root, ".storybook", "main.ts"), main("GLOBS"));
    expect(storyCensus(root)).toEqual({ suffixes: null, files: [] });
  });
});
