// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { filingFor } from "./projectrecord";

describe("which project a send files under", () => {
  const two = [{ id: "p-1" }, { id: "p-2" }];

  it("takes the thread's project first, because a conversation is one body of work", () => {
    expect(
      filingFor({
        threadProjectId: "p-thread",
        dealProjectId: "p-deal",
        reachable: two,
      }),
    ).toEqual({ kind: "derived", projectId: "p-thread", from: "thread" });
  });

  it("falls back to the deal's project when the thread carries none", () => {
    // The case that sent this work: a deal whose project was attached after
    // the conversation had already started, so the old mail names no project.
    expect(filingFor({ dealProjectId: "p-deal", reachable: two })).toEqual({
      kind: "derived",
      projectId: "p-deal",
      from: "deal",
    });
  });

  it("asks only when several are in reach and nothing names one", () => {
    expect(filingFor({ reachable: two })).toEqual({ kind: "choose" });
  });

  it("says nothing when there is no project at all", () => {
    expect(filingFor({ reachable: [] })).toEqual({ kind: "none" });
  });

  it("does not ask about a single reachable project", () => {
    // A question with one answer is not a question. Nothing names it, so
    // nothing is stated either — the rep is simply not interrupted.
    expect(filingFor({ reachable: [{ id: "p-1" }] })).toEqual({ kind: "none" });
  });
});
