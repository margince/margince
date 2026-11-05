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

  it("is empty for an ARCHIVED project, whose key may already be somebody else's", () => {
    // A key is unique among LIVE projects only. Stamping an archived one would
    // route the customer's reply to whichever project holds that key now.
    expect(subjectTag(project({ archived_at: "2026-08-01T00:00:00Z" }))).toBe(
      "",
    );
  });

  it("clears another project's tag rather than adding a second one", () => {
    // Two bracketed keys make the inbound matcher ambiguous, so it resolves to
    // NEITHER — a subject carrying both files nowhere.
    expect(withSubjectTag("[OTHER-9] Re: Hallo", "[N2P-1]")).toBe(
      "[N2P-1] Re: Hallo",
    );
  });

  it("clears a key tag the rep moved into the middle of the subject", () => {
    expect(withSubjectTag("Re: [OTHER-9] Hallo", "[N2P-1]")).toBe(
      "[N2P-1] Re: Hallo",
    );
  });

  it("leaves a bracketed group the matcher could never read as a key", () => {
    // The letter-led rule is what excludes a bare number, so `[2026]` is a year
    // and stays. `[FYI]` is deliberately NOT in this test: it is key-shaped to
    // the capture matcher too, so clearing it is correct — a subject carrying
    // it alongside a real key would file nowhere.
    expect(withSubjectTag("[2026] Budget", "[N2P-1]")).toBe(
      "[N2P-1] [2026] Budget",
    );
  });

  it("leaves the rest of the subject byte for byte", () => {
    // The tag is re-applied on every edit while a project is chosen, so this
    // runs over text the rep is actively typing. Anything it tidies beyond the
    // tag is their writing being rewritten under the cursor.
    expect(withSubjectTag("[N2P-1] Re:  Kurzer Austausch?", "[N2P-1]")).toBe(
      "[N2P-1] Re:  Kurzer Austausch?",
    );
    // Including a trailing space, which is the one they just pressed before
    // typing the next word.
    expect(withSubjectTag("[N2P-1] Re: ", "[N2P-1]")).toBe("[N2P-1] Re: ");
    // And runs of spaces nowhere near a tag.
    expect(withSubjectTag("Re:   viel   Platz", "[N2P-1]")).toBe(
      "[N2P-1] Re:   viel   Platz",
    );
  });

  it("keeps a second space the rep typed right after the tag", () => {
    // The space beside the tag is the easiest one to lose and the least
    // visible when it goes: what this module manages is the tag plus ONE
    // separator, and anything past that is the rep's own spacing.
    expect(withSubjectTag("[N2P-1]  Re: Hallo", "[N2P-1]")).toBe(
      "[N2P-1]  Re: Hallo",
    );
    expect(withSubjectTag("[N2P-1]   Re: Hallo", "[N2P-1]")).toBe(
      "[N2P-1]   Re: Hallo",
    );
    // And the ordinary single-separator case is untouched, so re-applying is
    // still idempotent rather than growing a space each pass.
    expect(withSubjectTag("[N2P-1] Re: Hallo", "[N2P-1]")).toBe(
      "[N2P-1] Re: Hallo",
    );
  });

  it("closes the gap a removed tag leaves behind", () => {
    // One separator survives when the tag sat between two things, so a subject
    // does not read "Re:  Hallo" with a hole where the tag was.
    expect(withSubjectTag("Re: [OTHER-9] Hallo", "[N2P-1]")).toBe(
      "[N2P-1] Re: Hallo",
    );
  });

  it("clears a key carrying the underscore the CHECK admits", () => {
    // project_key_shape is `^[A-Za-z][A-Za-z0-9_-]{1,23}$`, so `alpha_two` is a
    // key the capture matcher reads. Left standing it is a SECOND live key
    // beside the one written here — and a subject naming two files under
    // neither, which is the exact outcome this function exists to prevent.
    expect(withSubjectTag("[alpha_two] Hallo", "[N2P-1]")).toBe(
      "[N2P-1] Hallo",
    );
  });

  it("takes no separator from the tag beside it, because that one is gone", () => {
    expect(withSubjectTag("[OTHER-9] [THIRD-2] Hallo", "[N2P-1]")).toBe(
      "[N2P-1] Hallo",
    );
  });

  it("takes the leading space of a key tag that ends the subject", () => {
    expect(withSubjectTag("Hallo [OTHER-9]", "[N2P-1]")).toBe("[N2P-1] Hallo");
  });

  it("leaves a bracket that never closes, as the capture side does", () => {
    // A subject trailing off mid-marker names no key. Reading the rest of the
    // line as one would turn a stray `[` into a match.
    expect(withSubjectTag("[N2P-1 Hallo", "[N2P-1]")).toBe(
      "[N2P-1] [N2P-1 Hallo",
    );
  });

  it("reads a group from its bracket to the NEXT close, as capture does", () => {
    // `a[N2P-1` is what the matcher sees inside this group, and it is not a
    // key — so neither side reads one here, and the rep's line stands. A
    // frontend that instead started the group at the INNER `[` would strip a
    // tag the matcher never saw, and leave the `[a` in front of it behind.
    expect(withSubjectTag("[a[N2P-1] Hallo", "[OTHER-9]")).toBe(
      "[OTHER-9] [a[N2P-1] Hallo",
    );
  });

  it("reads a run of brackets after the last close without rewriting it", () => {
    // Every `[` here opens a group that never closes, which is the shape a
    // backtracking bracket matcher re-reads the tail of the line for, once per
    // bracket. A subject is attacker-supplied text, and none of these is a key:
    // the line comes back whole.
    expect(withSubjectTag("Re: ] [[[[[[[[", "[N2P-1]")).toBe(
      "[N2P-1] Re: ] [[[[[[[[",
    );
  });

  it("takes its own tag back off, and nobody else's", () => {
    expect(stripSubjectTag("[N2P-1] Re: Hallo", "[N2P-1]")).toBe("Re: Hallo");
    // One separator comes off with the tag, not every space behind it.
    expect(stripSubjectTag("[N2P-1]  Re: Hallo", "[N2P-1]")).toBe(" Re: Hallo");
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
