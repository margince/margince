// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A story that loads vitest breaks only under `storybook dev`, which no lane
// runs. The static build renders it, so fe-uat stays green over the defect.

import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import {
  edgesOf,
  loadsTestRunner,
  testRunnerReach,
} from "../../scripts/lib/import-reach";
import { isDocsPage, storySuffixes } from "../../scripts/lib/story-files";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  frontendRoot,
} from "../tree";

// Read off Storybook's own globs, so a new kind of story file joins the corpus.
const mainPath = join(frontendRoot, ".storybook", "main.ts");
const suffixes = storySuffixes(mainPath, readFileSync(mainPath, "utf8"));

const PREVIEW = ".storybook/preview.tsx";

const loadedByStorybook = (file: CensusFile) =>
  (file.area === "src" &&
    (suffixes ?? []).some((suffix) => file.path.endsWith(suffix))) ||
  (file.area === "storybook" && file.rel === PREVIEW);

function componentBeside(story: string, tree: CensusTree): string | undefined {
  return [".tsx", ".ts"]
    .map((extension) => story.replace(/\.stories\.tsx?$/, extension))
    .find((file) => file !== story && tree.at(file) !== undefined);
}

// More than half of the stories with a component beside them must reach it,
// or the import walk sees no edges and every story passes.
function edgesAreRead(
  corpus: readonly CensusFile[],
  tree: CensusTree,
): boolean {
  const paired = corpus
    .filter((file) => file.area === "src" && !isDocsPage(file.path))
    .flatMap((story) => {
      const component = componentBeside(story.path, tree);
      return component === undefined ? [] : [{ story, component }];
    });
  const reached = paired.filter(({ story, component }) =>
    edgesOf(story.path, "scanned").includes(component),
  );
  return reached.length > paired.length / 2;
}

const TEST_FILE = /\.test\.tsx?$/;

export const rule: CensusRule = {
  name: "stories-load-no-vitest",
  corpus: (tree) => tree.files.filter(loadedByStorybook),
  floors: (corpus, tree) => [
    ...(suffixes === null
      ? ["the story globs in .storybook/main.ts could not be read"]
      : []),
    ...(corpus.some((file) => file.rel === PREVIEW)
      ? []
      : [`${PREVIEW} is not in the corpus`]),
    ...(tree.files.some(
      (file) =>
        file.area === "src" &&
        TEST_FILE.test(file.path) &&
        loadsTestRunner(file.path),
    )
      ? []
      : ["no test file under src/ was seen to load vitest"]),
    ...(edgesAreRead(corpus, tree)
      ? []
      : ["fewer than half the stories reach the component beside them"]),
  ],
  across: (corpus) =>
    testRunnerReach(corpus.map((file) => file.path)).map((path) =>
      path.map((file) => relative(frontendRoot, file)).join(" -> "),
    ),
  advice:
    "Each path runs from a story to a module that imports vitest. Move the " +
    "helper that needs vitest into a *.testkit.ts no story imports.",
};
