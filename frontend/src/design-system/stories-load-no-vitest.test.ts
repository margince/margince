// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A story that loads vitest breaks only under `storybook dev`, which no lane
// runs; the static build renders it, so fe-uat stays green over the defect.

import { existsSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  importPathTo,
  loadsTestRunner,
  testRunnerReach,
} from "../../scripts/lib/import-reach";
import { filesUnder } from "../../scripts/lib/source-tree";
import { isDocsPage, storyCensus } from "../../scripts/lib/story-files";

const frontendRoot = resolve(__dirname, "..", "..");
const census = storyCensus(frontendRoot);
const preview = join(frontendRoot, ".storybook", "preview.tsx");

function componentBeside(story: string): string | undefined {
  return [".tsx", ".ts"]
    .map((extension) => story.replace(/\.stories\.tsx?$/, extension))
    .find((file) => file !== story && existsSync(file));
}

describe("no file Storybook loads reaches vitest", () => {
  it("reads a real story census, real vitest modules and real import edges", () => {
    expect(census.suffixes).not.toBeNull();
    expect(census.files.length).toBeGreaterThan(0);
    expect(
      filesUnder(join(frontendRoot, "src")).filter(loadsTestRunner).length,
    ).toBeGreaterThan(0);
    const paired = census.files
      .filter((file) => !isDocsPage(file))
      .flatMap((story) => {
        const component = componentBeside(story);
        return component === undefined ? [] : [{ story, component }];
      });
    const reached = paired.filter(
      ({ story, component }) =>
        importPathTo(story, new Set([component]), "values") !== null,
    );
    expect(reached.length).toBeGreaterThan(paired.length / 2);
  });

  it("no story, docs page or preview reaches a value import of vitest", () => {
    const paths = testRunnerReach([...census.files, preview]).map((path) =>
      path.map((file) => relative(frontendRoot, file)).join(" -> "),
    );
    expect(
      paths,
      "Each path runs from a story to a module that imports vitest. Move the " +
        "helper that needs vitest into a *.testkit.ts no story imports.",
    ).toEqual([]);
  });
});
