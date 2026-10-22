// The render gate's (fe-uat.mjs) rules, asked of the rules themselves: the gate
// is a script that talks to git and a browser as it loads.

import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { FAILURE_EVENTS, outcomeErrors } from "./lib/story-outcome.mjs";
import {
  changesSince,
  needsStory,
  scopeFor,
  storiesInScope,
  uatVerdict,
  unresolvedStoryFiles,
} from "./lib/uat-scope.mjs";
import {
  drainInOrder,
  renewingBroken,
  requestedWorkers,
  workersFor,
} from "./lib/uat-workers.mjs";

// A skip keyed too widely reports PASS over a component that lost its story, so
// the rule staying closed is the half worth a test.
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
    expect(needsStory("frontend/src/lib/router.ts")).toBe(false);
    expect(needsStory("frontend/src/api/schema.d.ts")).toBe(false);
  });
});

describe("scopeFor", () => {
  it("renders what the diff reaches unless asked for every story", () => {
    expect(scopeFor(["node", "fe-uat.mjs"])).toBe("diff");
    expect(scopeFor(["node", "fe-uat.mjs", "--allow-missing"])).toBe("diff");
    expect(scopeFor(["node", "fe-uat.mjs", "--all-stories"])).toBe("all");
  });
});

describe("storiesInScope", () => {
  const index = [
    { id: "a--one", type: "story", importPath: "./src/a.stories.tsx" },
    { id: "a--two", type: "story", importPath: "./src/a.stories.tsx" },
    { id: "b--one", type: "story", importPath: "./src/b.stories.tsx" },
    { id: "intro--docs", type: "docs", importPath: "./src/intro.mdx" },
  ];
  const ids = (entries) => entries.map((entry) => entry.id);

  it("takes every story the build registered, and no docs page", () => {
    expect(ids(storiesInScope(index, "all", new Set()))).toEqual([
      "a--one",
      "a--two",
      "b--one",
    ]);
  });

  it("takes only the story files the diff reached", () => {
    expect(
      ids(storiesInScope(index, "diff", new Set(["./src/a.stories.tsx"]))),
    ).toEqual(["a--one", "a--two"]);
  });
});

describe("unresolvedStoryFiles", () => {
  const stories = [
    { id: "a--one", type: "story", importPath: "./src/a.stories.tsx" },
  ];

  it("reports a story file on disk the build registered no story from", () => {
    expect(
      unresolvedStoryFiles(
        new Set(["./src/a.stories.tsx", "./src/dropped.stories.tsx"]),
        stories,
      ),
    ).toEqual(["./src/dropped.stories.tsx"]);
  });

  it("reports nothing when every story file registered", () => {
    expect(
      unresolvedStoryFiles(new Set(["./src/a.stories.tsx"]), stories),
    ).toEqual([]);
  });
});

describe("uatVerdict", () => {
  const clean = {
    results: [{ id: "a--one", pass: true }],
    unresolved: [],
    missing: [],
    allowMissing: false,
  };

  it("fails an all-stories run that captured nothing", () => {
    expect(uatVerdict({ ...clean, scope: "all", results: [] })).toEqual({
      pass: false,
      emptyCensus: true,
    });
  });

  it("passes a diff run that captured nothing and misses nothing", () => {
    expect(uatVerdict({ ...clean, scope: "diff", results: [] })).toEqual({
      pass: true,
      emptyCensus: false,
    });
  });

  it.each(["all", "diff"])(
    "fails an unregistered story file in the %s scope",
    (scope) => {
      expect(
        uatVerdict({ ...clean, scope, unresolved: ["./src/x.stories.tsx"] })
          .pass,
      ).toBe(false);
    },
  );

  it("fails a story that did not render clean", () => {
    expect(
      uatVerdict({
        ...clean,
        scope: "all",
        results: [{ id: "a--one", pass: false }],
      }).pass,
    ).toBe(false);
  });

  it("lets allowMissing excuse a missing story and nothing else", () => {
    const missing = [{ component: "frontend/src/x.tsx" }];
    expect(uatVerdict({ ...clean, scope: "diff", missing }).pass).toBe(false);
    expect(
      uatVerdict({ ...clean, scope: "diff", missing, allowMissing: true }).pass,
    ).toBe(true);
    expect(
      uatVerdict({
        ...clean,
        scope: "diff",
        allowMissing: true,
        unresolved: ["./src/x.stories.tsx"],
      }).pass,
    ).toBe(false);
    expect(
      uatVerdict({
        ...clean,
        scope: "diff",
        allowMissing: true,
        results: [{ id: "a--one", pass: false }],
      }).pass,
    ).toBe(false);
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
    const overlaps = [];
    await drainInOrder(
      [0, 1, 2, 3, 4, 5],
      ["a", "b", "c"],
      async (item, lane) => {
        if (busy.has(lane)) overlaps.push(`${lane} took ${item} while busy`);
        busy.add(lane);
        used.add(lane);
        await settleAfter(item + 1);
        busy.delete(lane);
      },
      () => "recovered",
    );
    expect(overlaps).toEqual([]);
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

const quiet = Object.fromEntries(FAILURE_EVENTS.map((event) => [event, null]));

describe("outcomeErrors", () => {
  it("passes a story whose channel reported no failure", () => {
    expect(outcomeErrors(quiet)).toEqual([]);
  });

  // The rejection is on the channel even when no console line reached the gate
  // before the screenshot.
  it("fails a play() that threw, once, on its first line", () => {
    const error = {
      name: "TestingLibraryElementError",
      message: "Unable to find an element\n\n<body>…</body>",
    };
    expect(
      outcomeErrors({
        ...quiet,
        playFunctionThrewException: [error],
        storyThrewException: [error],
      }),
    ).toEqual([
      "play() threw TestingLibraryElementError: Unable to find an element",
    ]);
  });

  it("fails a render that threw or errored without a play()", () => {
    expect(
      outcomeErrors({
        ...quiet,
        storyThrewException: [{ name: "TypeError", message: "x is undefined" }],
        storyErrored: [{ title: "No component", description: "export one" }],
      }),
    ).toEqual([
      "the story threw TypeError: x is undefined",
      "the story errored: No component — export one",
    ]);
  });

  it("fails each error left unhandled while playing", () => {
    expect(
      outcomeErrors({
        ...quiet,
        unhandledErrorsWhilePlaying: [[{ message: "a" }, { message: "b" }]],
      }),
    ).toEqual([
      "unhandled error while playing: a",
      "unhandled error while playing: b",
    ]);
  });

  // A module that throws at import emits nothing but this, with or without the
  // story it could not load.
  it("fails a story Storybook could not load", () => {
    expect(
      outcomeErrors({ ...quiet, storyMissing: ["compose--default"] }),
    ).toEqual([
      "Storybook could not load the story (compose--default): its module failed to import or no longer exports it",
    ]);
    expect(outcomeErrors({ ...quiet, storyMissing: [] })).toEqual([
      "Storybook could not load the story: its module failed to import or no longer exports it",
    ]);
  });
});

describe("renewingBroken", () => {
  it("gives a broken lane a fresh resource, so one crash fails one item", async () => {
    const lanes = [
      { name: "a", broken: false, renewals: 0 },
      { name: "b", broken: false, renewals: 0 },
    ];
    const results = await drainInOrder(
      [0, 1, 2, 3, 4, 5],
      lanes,
      renewingBroken(
        async (item, lane) => {
          if (lane.broken) throw new Error("page crashed");
          if (item === 1) {
            lane.broken = true;
            throw new Error("page crashed");
          }
          return await settleAfter(1, `ok ${item}`);
        },
        {
          isBroken: (lane) => lane.broken,
          renew: async (lane) => {
            lane.broken = false;
            lane.renewals++;
          },
        },
      ),
      (item, error) => `failed ${item}: ${error.message}`,
    );
    expect(results).toEqual([
      "ok 0",
      "failed 1: page crashed",
      "ok 2",
      "ok 3",
      "ok 4",
      "ok 5",
    ]);
    expect(lanes.reduce((sum, lane) => sum + lane.renewals, 0)).toBe(1);
  });
});

it("includes committed, staged, unstaged and new stories before commit", () => {
  const dir = mkdtempSync(join(tmpdir(), "approval-uat-scope-"));
  const git = (args) =>
    execFileSync("git", args.split(" "), { cwd: dir, encoding: "utf8" }).trim();
  const write = (name, text) => writeFileSync(join(dir, name), text);
  try {
    git("init --quiet");
    git("config user.name Fixture");
    git("config user.email fixture@example.test");
    for (const name of [
      "committed.tsx",
      "staged.tsx",
      "unstaged.tsx",
      "removed.tsx",
    ])
      write(name, "before");
    write(".gitignore", "ignored.tsx\n");
    git("add .");
    git("-c core.hooksPath=/dev/null commit --quiet -m fixture");
    const base = git("rev-parse HEAD");
    write("committed.tsx", "committed change");
    git("add committed.tsx");
    git("-c core.hooksPath=/dev/null commit --quiet -m changed");
    write("staged.tsx", "staged change");
    git("add staged.tsx");
    write("unstaged.tsx", "working change");
    write("new.stories.tsx", "new story");
    write("ignored.tsx", "build output");
    rmSync(join(dir, "removed.tsx"));
    expect(changesSince(git, base).sort()).toEqual([
      "committed.tsx",
      "new.stories.tsx",
      "staged.tsx",
      "unstaged.tsx",
    ]);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
