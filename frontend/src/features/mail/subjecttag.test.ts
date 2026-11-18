// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { stripSubjectTag, subjectTag, withSubjectTag } from "./subjecttag";

type Project = components["schemas"]["Project"];

const project = (over: Partial<Project> = {}): Project =>
  ({ id: "p-1", name: "Netcare 2 project", key: "N2P-1", ...over }) as Project;

describe("the project tag a subject carries", () => {
  it("brackets the key the capture side reads back", () => {
    expect(subjectTag(project())).toBe("[N2P-1]");
  });

  it("is empty for a project with no key, rather than an empty bracket", () => {
    // `[]` is not a key reference, and the inbound matcher reads nothing from
    // it. Writing one would only add noise to the subject.
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
    // neither. A subject carrying both files nowhere.
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
    // The letter-led rule excludes a bare number, so `[2026]` is a year and
    // stays. `[FYI]` is left out here: it is key-shaped to the capture matcher
    // too, so clearing it is correct. Beside a real key it would file nowhere.
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
    // The space beside the tag is the easiest one to lose unseen. This module
    // manages the tag plus one separator. Anything past that is the rep's.
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
    // project_key_shape admits the underscore, so `alpha_two` is a key the
    // capture matcher reads. Left standing it is a second live key beside the
    // one written here. A subject naming two files under neither.
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
    // key. So neither side reads one here, and the rep's line stands. Starting
    // the group at the inner `[` would strip a tag the matcher never saw.
    expect(withSubjectTag("[a[N2P-1] Hallo", "[OTHER-9]")).toBe(
      "[OTHER-9] [a[N2P-1] Hallo",
    );
  });

  it("reads a run of brackets after the last close without rewriting it", () => {
    // Every `[` here opens a group that never closes. A backtracking matcher
    // re-reads the tail of the line once per bracket on this shape. A subject
    // is attacker-supplied text, and none of these is a key: it comes back whole.
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
