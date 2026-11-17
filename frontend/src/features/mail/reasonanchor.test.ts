/** @vitest-environment happy-dom */
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { reasonEvidence, useReasonForAnchor } from "./reasonanchor";

describe("useReasonForAnchor", () => {
  it("clears the reason and its meeting when the composer moves to another anchor", () => {
    const fields = { setContext: vi.fn(), setMeetingEvidence: vi.fn() };
    const { rerender } = renderHook(
      ({ anchor }: { anchor: string | undefined }) =>
        useReasonForAnchor(anchor, fields),
      { initialProps: { anchor: "act-1" as string | undefined } },
    );
    expect(fields.setContext).not.toHaveBeenCalled();

    rerender({ anchor: "act-2" });
    expect(fields.setContext).toHaveBeenCalledWith("");
    expect(fields.setMeetingEvidence).toHaveBeenCalledWith(undefined);
  });
});

describe("reasonEvidence", () => {
  it("names the deal and the meeting a reason rests on, and nothing when neither", () => {
    expect(reasonEvidence("deal-1", "meeting-1")).toEqual({
      deal_id: "deal-1",
      activity_id: "meeting-1",
    });
    expect(reasonEvidence(undefined, undefined)).toBeUndefined();
  });
});
