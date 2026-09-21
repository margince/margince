// What the render gate (fe-uat.mjs) counts as a shipped component, asked of the
// rule itself.
//
// The failure that prompted the location rule: the gate reported
// `src/app/testing/shellharness.tsx` as "a changed component no story renders".
// It is a test harness — it mounts the shell for a suite and exports the queries
// those tests ask it — so the story it demanded would have documented nothing a
// reader ships, and the only ways past a gate like that are a waiver flag or a
// story written to satisfy it.
//
// The rule has to stay CLOSED in the other direction, which is the half worth a
// test: a skip keyed too widely reports PASS over a component that lost its
// story, and no assertion fails to say so.

import { describe, expect, it } from "vitest";
import { needsStory } from "./lib/uat-scope.mjs";

describe("needsStory", () => {
  it("skips a harness by the directory it sits in", () => {
    expect(needsStory("frontend/src/app/testing/x.tsx")).toBe(false);
    expect(needsStory("frontend/src/testing/x.tsx")).toBe(false);
  });

  it("asks for a story from a component beside that directory", () => {
    expect(needsStory("frontend/src/app/x.tsx")).toBe(true);
  });

  // The segment is the whole rule: a component whose name merely begins with
  // those letters is not a harness, and reading it as one is the shape of skip
  // that fails by passing.
  it("reads a whole path segment, not a prefix", () => {
    expect(needsStory("frontend/src/screens/testingground.tsx")).toBe(true);
    expect(needsStory("frontend/src/app/nottesting/x.tsx")).toBe(true);
  });

  it("skips a test, testkit or story by name", () => {
    expect(needsStory("frontend/src/app/x.test.tsx")).toBe(false);
    expect(needsStory("frontend/src/app/x.testkit.tsx")).toBe(false);
    expect(needsStory("frontend/src/app/x.stories.tsx")).toBe(false);
  });

  // Only a renderable module can have a story, so the gate asks nothing of a
  // plain module or a declaration file.
  it("asks nothing of a file no story could render", () => {
    expect(needsStory("frontend/src/app/router.ts")).toBe(false);
    expect(needsStory("frontend/src/api/schema.d.ts")).toBe(false);
  });
});
