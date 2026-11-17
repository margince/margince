// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it, vi } from "vitest";
import type { CommunicationContext } from "../../lib/compose-context";
import { fillFromDraft } from "./composedraftcall";

function formWith(context: CommunicationContext | "") {
  return {
    subject: "",
    body: "",
    toEmpty: false,
    setSubject: vi.fn(),
    setBody: vi.fn(),
    setServedBody: vi.fn(),
    setTo: vi.fn(),
    setDraftRef: vi.fn(),
    setProvenance: vi.fn(),
    setReasoning: vi.fn(),
    setScope: vi.fn(),
    context,
    setContext: vi.fn(),
    setMeetingEvidence: vi.fn(),
  };
}

const moved = {
  available: true as const,
  draft: {
    subject: "Re: Thursday",
    body: "Could we move our meeting to Friday?",
    to: [],
    communication_context: "requested_followup" as const,
    evidence: { activity_id: "meeting-1" },
  },
};

describe("fillFromDraft", () => {
  it("takes the drafted reason and its meeting when none was picked", () => {
    const form = formWith("");
    fillFromDraft(moved, form);
    expect(form.setContext).toHaveBeenCalledWith("requested_followup");
    expect(form.setMeetingEvidence).toHaveBeenCalledWith("meeting-1");
  });

  it("never replaces a reason the sender picked", () => {
    const form = formWith("marketing");
    fillFromDraft(moved, form);
    expect(form.setContext).not.toHaveBeenCalled();
    expect(form.setMeetingEvidence).not.toHaveBeenCalled();
  });

  it("leaves the reason alone when the draft names none", () => {
    const form = formWith("");
    fillFromDraft(
      { available: true, draft: { subject: "Hi", body: "Thanks", to: [] } },
      form,
    );
    expect(form.setContext).not.toHaveBeenCalled();
  });
});
