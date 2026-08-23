// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  filingFor,
  stripSubjectTag,
  subjectTag,
  withSubjectTag,
} from "./projectrecord";
import type { Project } from "./projects.form";

const project = (over: Partial<Project> = {}): Project =>
  ({ id: "p-1", name: "Netcare 2 project", key: "N2P-1", ...over }) as Project;

describe("the project tag a subject carries", () => {
  it("brackets the key the capture side reads back", () => {
    expect(subjectTag(project())).toBe("[N2P-1]");
  });

  it("is empty for a project with no key, rather than an empty bracket", () => {
    // `[]` is not a key reference, and the inbound matcher reads nothing from
    // it — writing one would be noise in the subject that buys nothing.
    expect(subjectTag(project({ key: null }))).toBe("");
    expect(subjectTag(null)).toBe("");
  });

  it("puts the tag at the front of the subject", () => {
    expect(withSubjectTag("Re: Kurzer Austausch?", "[N2P-1]")).toBe(
      "[N2P-1] Re: Kurzer Austausch?",
    );
  });

  it("does not stack when the subject already carries the tag", () => {
    // The composer re-derives the subject whenever the filing changes, so this
    // runs more than once over the same text.
    const once = withSubjectTag("Re: Kurzer Austausch?", "[N2P-1]");
    expect(withSubjectTag(once, "[N2P-1]")).toBe(once);
  });

  it("tags an empty subject without leaving a leading space", () => {
    expect(withSubjectTag("", "[N2P-1]")).toBe("[N2P-1]");
  });

  it("leaves the subject alone when there is no tag to add", () => {
    expect(withSubjectTag("Kurzer Austausch?", "")).toBe("Kurzer Austausch?");
  });

  it("takes its own tag back off, and nobody else's", () => {
    expect(stripSubjectTag("[N2P-1] Re: Hallo", "[N2P-1]")).toBe("Re: Hallo");
    // A different project's tag is the rep's text, not ours to remove.
    expect(stripSubjectTag("[OTHER-9] Re: Hallo", "[N2P-1]")).toBe(
      "[OTHER-9] Re: Hallo",
    );
    // A tag in the middle is prose, not a prefix we wrote.
    expect(stripSubjectTag("Re: [N2P-1] Hallo", "[N2P-1]")).toBe(
      "Re: [N2P-1] Hallo",
    );
  });
});

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
