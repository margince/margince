import { describe, expect, it } from "vitest";
import cases from "../../../../backend/internal/compose/contact360/testdata/claimstandsalone.json";
import type { components } from "../../api/schema";
import { claimStandsAlone } from "./contactpromises";

type ConversationClaim = components["schemas"]["ConversationClaim"];

// The client twin of claimStandsAlone in the contact360 compose package. Both
// read one table, so the page count and the server ranking keep one rule.
describe("claimStandsAlone", () => {
  it("reads a non-empty shared table", () => {
    expect(cases.length).toBeGreaterThan(0);
  });

  it.each(cases)("$name", ({ task, task_open, listed, stands_alone }) => {
    const claim: ConversationClaim = {
      id: "c-1",
      kind: "commitment_ours",
      body: "Send the quote",
      source_activity_id: "a-1",
      source_quote: "I will send the quote.",
      occurred_precision: "instant",
      status: "open",
      needs_review: false,
      task_activity_id: task,
      ...(task_open === null ? {} : { task_open }),
    };
    expect(claimStandsAlone(claim, new Set(listed))).toBe(stands_alone);
  });
});
