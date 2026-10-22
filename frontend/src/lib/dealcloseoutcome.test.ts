import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { dealCloseOutcome } from "./approvalpreview";

type Preview = components["schemas"]["ApprovalPreview"];

const previewLanding = (status: string): Preview => ({
  version: 1,
  state: "ready",
  reviewer_reason: "record_authority",
  changes: [
    {
      field: "status",
      before: { kind: "text", text: "open" },
      after: { kind: "text", text: status },
    },
  ],
});

// Whether a deal move closes the deal decides the card's label. A reopen must
// never read as a close, and a decided close must not read as "advance".
describe("dealCloseOutcome", () => {
  it("takes a pending move's landing from its preview", () => {
    expect(dealCloseOutcome(previewLanding("lost"))).toBe("lost");
    expect(dealCloseOutcome(previewLanding("won"))).toBe("won");
  });

  it("ignores a leftover lost reason when the preview lands open", () => {
    expect(
      dealCloseOutcome(previewLanding("open"), { lost_reason: "Budget cut" }),
    ).toBeUndefined();
  });

  it("reads a decided close from the lost reason the move carried", () => {
    expect(dealCloseOutcome(null, { lost_reason: "No-bid" })).toBe("lost");
  });

  it("reads the lost reason inside a REST-staged move", () => {
    expect(
      dealCloseOutcome(null, {
        operation: "advanceDeal",
        path: "/v1/deals/d-1/advance",
        body: { lost_reason: "No-bid" },
      }),
    ).toBe("lost");
  });

  it("says nothing about a decided move with no lost reason", () => {
    expect(dealCloseOutcome(null, { to_stage_id: "s-1" })).toBeUndefined();
    expect(dealCloseOutcome(null, { lost_reason: "  " })).toBeUndefined();
  });
});
