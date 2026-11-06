// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  OTHER_KINDS,
  SOURCE_ANSWERS,
  SOURCE_KINDS,
  sourceKindFor,
} from "./sourcequestion";

describe("the source question maps each answer to an existing kind", () => {
  // Agreeing to be contacted does not show they were told who holds their
  // details, so no answer in the first list closes the Art. 13 duty that way.
  it("keeps the notice duty open for a face-to-face meeting", () => {
    expect(sourceKindFor("met", "")).toBe("event_or_form");
    const first = SOURCE_ANSWERS.map((answer) => sourceKindFor(answer, ""));
    expect(first).not.toContain("in_person_permission");
    expect(OTHER_KINDS).toContain("in_person_permission");
  });

  it("keeps contacting us apart from asking for a quote or a meeting", () => {
    expect(sourceKindFor("contacted", "")).toBe("subject_initiated");
    expect(sourceKindFor("asked", "")).toBe("requested_quote_or_meeting");
  });

  it("saves nothing for Not sure, or for Another source with no pick", () => {
    expect(sourceKindFor("unsure", "")).toBeNull();
    expect(sourceKindFor("unsure", "referral")).toBeNull();
    expect(sourceKindFor("other", "")).toBeNull();
    expect(sourceKindFor("", "")).toBeNull();
    expect(sourceKindFor("other", "crm_migration")).toBe("crm_migration");
  });

  // Every kind a reviewer may confirm is reachable, so the question never
  // hides a source the old picker offered.
  it("reaches every confirmable kind, with seven behind Another source", () => {
    const reached = new Set([
      ...SOURCE_ANSWERS.map((answer) => sourceKindFor(answer, "")),
      ...OTHER_KINDS,
    ]);
    reached.delete(null);
    expect([...reached].sort()).toEqual([...SOURCE_KINDS].sort());
    expect(OTHER_KINDS).toHaveLength(7);
    expect(SOURCE_KINDS).not.toContain("unknown_legacy");
  });
});
