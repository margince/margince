/** @vitest-environment jsdom */

// That a case which leaves `GET /me` unrouted FAILS, rather than warning.
//
// The fetch stub refuses a session it cannot guess, and a refused session reads
// as a malformed one: every capability hook fails closed and the surface draws
// its denied branch. Whether a case asserted against that branch or the one it
// was named for came down to which settled first, so it passed alone and failed
// under load on a different name each run. vitest.setup.ts turns the refusal
// into a failure, and this is what fails if that stops.
//
// Behavioural rather than a check that the hook exists, for the same reason
// autocleanup.test.ts is: nothing reports a hook that was never registered, and
// a suite whose sessions are all denied mostly still passes.
//
// `it.fails` is the whole instrument — it passes only when the case beneath it
// is reported failing, which under an unarmed setup it would not be.

import { expect, it } from "vitest";
import { installFetchStub } from "../src/screens/story-utils";
import { takeUnroutedSessionProbes } from "../src/screens/unrouted-session";

it.fails("fails a case that asked for a session the stub was not given", async () => {
  installFetchStub({});

  const refused = await fetch("/v1/me");

  // The case's own body is sound: the refusal is a 501 and says so. What must
  // fail is the case, after it, for having asked at all.
  expect(refused.status).toBe(501);
});

it("hands the next case a clean count", () => {
  // The guard fires once per case or it fires forever: an unclaimed probe that
  // survived the afterEach would fail every case after it, which is a suite
  // nobody can read.
  expect(takeUnroutedSessionProbes()).toBe(0);
});
