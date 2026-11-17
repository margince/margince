// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it, vi } from "vitest";
import type { CommunicationContext } from "../../lib/compose-context";
import { fillFromDraft } from "./composedraftcall";

function formWith(
  context: CommunicationContext | "",
  reasonFromDraft = false,
  fromInstruction = true,
) {
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
    fromInstruction,
    context,
    reasonFromDraft,
    setContext: vi.fn(),
    setMeetingEvidence: vi.fn(),
    setReasonFromDraft: vi.fn(),
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
    expect(form.setReasonFromDraft).toHaveBeenCalledWith(true);
  });

  it("drops a drafted reason when a later instruction names none", () => {
    const form = formWith("requested_followup", true);
    fillFromDraft(
      { available: true, draft: { subject: "Hi", body: "Thanks", to: [] } },
      form,
    );
    expect(form.setContext).toHaveBeenCalledWith("");
    expect(form.setMeetingEvidence).toHaveBeenCalledWith(undefined);
    expect(form.setReasonFromDraft).toHaveBeenCalledWith(false);
  });

  it("keeps a drafted reason through a rewrite verb", () => {
    const form = formWith("requested_followup", true, false);
    fillFromDraft(
      { available: true, draft: { subject: "Hi", body: "Shorter", to: [] } },
      form,
    );
    expect(form.setContext).not.toHaveBeenCalled();
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
