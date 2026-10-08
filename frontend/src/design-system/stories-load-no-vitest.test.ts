// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A story that loads vitest breaks only under `storybook dev`, which no lane
// runs; the static build renders it, so fe-uat stays green over the defect.

import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { testRunnerReach } from "../../scripts/lib/import-reach";
import { filesUnder } from "../../scripts/lib/source-tree";
import { isDocsPage, storyCensus } from "../../scripts/lib/story-files";

const frontendRoot = resolve(__dirname, "..", "..");
const census = storyCensus(frontendRoot);
const stories = census.files.filter((file) => !isDocsPage(file));
const { runners, reaches } = testRunnerReach(
  [...stories, join(frontendRoot, ".storybook", "preview.tsx")],
  filesUnder(join(frontendRoot, "src")),
);

describe("no file Storybook loads reaches vitest", () => {
  it("reads a real story census and a real set of vitest modules", () => {
    expect(census.suffixes).not.toBeNull();
    expect(stories.length).toBeGreaterThan(0);
    expect(runners.size).toBeGreaterThan(0);
  });

  it("reaches a value import of vitest from no story", () => {
    const paths = reaches.map((path) =>
      path.map((file) => relative(frontendRoot, file)).join(" -> "),
    );
    expect(
      paths,
      "Each path runs from a story to a module that imports vitest. Move the " +
        "helper that needs vitest into a *.testkit.ts no story imports.",
    ).toEqual([]);
  });
});
