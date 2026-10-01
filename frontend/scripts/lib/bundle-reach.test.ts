// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  buildScriptEntries,
  documentEntries,
  reachedModules,
  surfaceEntries,
} from "./bundle-reach";

// The entry points are the whole corpus: a census reading a set that came back
// EMPTY reports a clean tree from nothing, and every reader of this module
// inherits that. So the derivation is held to finding some of each kind, and to
// naming only files that exist.
//
// The walk parses the whole app, so it runs once at collection, as
// onboarding-dead-css.test.ts runs it: it has no waiter, and timing it against
// the suite's per-test budget (vitest.budget.ts) failed it on a busy runner.
const reached = reachedModules();

describe("the bundle's entry points", () => {
  it("derives an entry from every owner that declares one", () => {
    expect(documentEntries().length).toBeGreaterThan(1);
    expect(surfaceEntries().length).toBeGreaterThan(0);
    expect(buildScriptEntries().length).toBeGreaterThan(0);
  });

  it("reaches the app's own modules, not a handful of entries", () => {
    // Well below the real count: the floor is here to catch a walk that
    // stopped walking, not to track how many modules the app has.
    expect(reached.size).toBeGreaterThan(200);
  });
});
