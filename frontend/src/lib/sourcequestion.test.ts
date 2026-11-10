// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { stable } from "../format/collate";
import {
  OTHER_KINDS,
  SOURCE_ANSWERS,
  SOURCE_KINDS,
  sourceKindFor,
} from "./sourcequestion";

describe("the source question maps each answer to an existing kind", () => {
  // At a meeting they learned who we are, so it owes no notice, and an
  // event or form, which does, stays a separate answer.
  it("confirms a face-to-face meeting as permission given in person", () => {
    expect(sourceKindFor("met", "")).toBe("in_person_permission");
    expect(OTHER_KINDS).not.toContain("in_person_permission");
    expect(OTHER_KINDS).toContain("event_or_form");
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
  it("reaches every confirmable kind, with six behind Another source", () => {
    const reached = new Set([
      ...SOURCE_ANSWERS.map((answer) => sourceKindFor(answer, "")),
      ...OTHER_KINDS,
    ]);
    reached.delete(null);
    expect([...reached].sort((a, b) => stable(String(a), String(b)))).toEqual(
      [...SOURCE_KINDS].sort(stable),
    );
    expect(OTHER_KINDS).toHaveLength(6);
    expect(SOURCE_KINDS).not.toContain("unknown_legacy");
  });
});
