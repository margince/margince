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
import {
  drainInOrder,
  requestedWorkers,
  workersFor,
} from "./lib/uat-workers.mjs";

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

describe("requestedWorkers", () => {
  it("defaults to half the cores, at most four and at least one", () => {
    expect(requestedWorkers(undefined, 12)).toBe(4);
    expect(requestedWorkers(undefined, 6)).toBe(3);
    expect(requestedWorkers(undefined, 3)).toBe(1);
    expect(requestedWorkers(undefined, 1)).toBe(1);
  });

  it("takes FE_UAT_WORKERS over the default, above the ceiling too", () => {
    expect(requestedWorkers("1", 12)).toBe(1);
    expect(requestedWorkers("8", 2)).toBe(8);
  });

  it.each(["0", "-1", "1.5", "two", "", " 2", "08"])(
    "refuses %j, naming the variable and the value",
    (value) => {
      expect(() => requestedWorkers(value, 12)).toThrow(
        `FE_UAT_WORKERS must be a positive integer, got "${value}"`,
      );
    },
  );
});

describe("workersFor", () => {
  it("opens no more pages than there are stories", () => {
    expect(workersFor(4, 2)).toBe(2);
    expect(workersFor(4, 165)).toBe(4);
    expect(workersFor(1, 165)).toBe(1);
  });
});

// Settles after `turns` microtask hops, so a test can order completions without
// a clock.
async function settleAfter(turns, value) {
  for (let i = 0; i < turns; i++) await Promise.resolve();
  return value;
}

describe("drainInOrder", () => {
  it("returns results in item order when later items finish first", async () => {
    const finished = [];
    const results = await drainInOrder(
      [0, 1, 2, 3],
      ["a", "b"],
      async (item) => {
        const value = await settleAfter((4 - item) * 3, item * 10);
        finished.push(item);
        return value;
      },
      () => "recovered",
    );
    expect(finished).not.toEqual([0, 1, 2, 3]);
    expect(results).toEqual([0, 10, 20, 30]);
  });

  it("runs one item per lane at a time, on every lane", async () => {
    const busy = new Set();
    const used = new Set();
    await drainInOrder(
      [0, 1, 2, 3, 4, 5],
      ["a", "b", "c"],
      async (item, lane) => {
        expect(busy.has(lane)).toBe(false);
        busy.add(lane);
        used.add(lane);
        await settleAfter(item + 1);
        busy.delete(lane);
      },
      () => "recovered",
    );
    expect([...used].sort()).toEqual(["a", "b", "c"]);
  });

  it("with one lane, runs items strictly in order", async () => {
    const events = [];
    await drainInOrder(
      [0, 1, 2],
      ["only"],
      async (item) => {
        events.push(`start ${item}`);
        await settleAfter(3 - item);
        events.push(`end ${item}`);
      },
      () => "recovered",
    );
    expect(events).toEqual([
      "start 0",
      "end 0",
      "start 1",
      "end 1",
      "start 2",
      "end 2",
    ]);
  });

  it("records a throwing item through recover and keeps draining", async () => {
    const results = await drainInOrder(
      [0, 1, 2, 3, 4],
      ["a", "b"],
      async (item) => {
        if (item === 1) throw new Error("goto timed out");
        return await settleAfter(item, `ok ${item}`);
      },
      (item, error) => `failed ${item}: ${error.message}`,
    );
    expect(results).toEqual([
      "ok 0",
      "failed 1: goto timed out",
      "ok 2",
      "ok 3",
      "ok 4",
    ]);
  });
});
