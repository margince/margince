/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { messageText } from "../../design-system/richtext-testing";
import { draftButton } from "../../testing/composedraft.testkit";
import { ComposeModal } from "./compose";
import {
  jsonResponse,
  PROVISIONAL_VOICE_PROFILE,
  render,
  stubRoutes,
  WHY_ASK,
} from "./compose.testkit";

// An indigo verb ("Draft reply") promises that the model writes. The composer
// it opens keeps that promise itself, rather than offering the same verb again.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const DRAFT = "POST /activities/act-1/draft-email";

const drafted = {
  subject: "Re: Q3",
  body: "Tuesday works for us.",
  to: ["buyer@acme.test"],
  ai_generated: true,
  ai_disclosure: "Drafted with AI assistance. Review before sending.",
  voice_profile_version: 3,
  draft_ref: "vd-a",
};

function renderReply(draftOnOpen: boolean) {
  render(
    <ComposeModal
      activityId="act-1"
      entityType="contact"
      entityId="p-1"
      intent="reply to their last message"
      draftOnOpen={draftOnOpen}
      open
      onClose={vi.fn()}
    />,
  );
}

describe("ComposeModal draftOnOpen", () => {
  it("drafts once on open, steered by the intent it was given", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      [DRAFT]: () => jsonResponse(drafted),
    });
    renderReply(true);

    await waitFor(() =>
      expect(messageText("Body")).toBe("Tuesday works for us."),
    );
    const asks = sent.filter((r) => r.key === DRAFT);
    expect(asks).toHaveLength(1);
    expect(asks[0]?.body).toMatchObject({
      intent: "reply to their last message",
    });
  });

  it("waits for a press when the opener made no such promise", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      [DRAFT]: () => jsonResponse(drafted),
    });
    renderReply(false);

    await screen.findByRole("combobox", { name: WHY_ASK });
    await waitFor(() =>
      expect(draftButton().hasAttribute("disabled")).toBe(false),
    );
    expect(sent.filter((r) => r.key === DRAFT)).toHaveLength(0);
    expect(messageText("Body")).toBe("");
  });

  it("leaves a saved draft on screen instead of drafting over it", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "GET /mail-drafts": () =>
        jsonResponse({
          id: "md-1",
          anchor_type: "activity",
          anchor_id: "act-1",
          to: ["buyer@acme.test"],
          cc: [],
          bcc: [],
          subject: "Re: Q3",
          body: "Half written before lunch",
          html_body: "<p>Half written before lunch</p>",
          version: 3,
          agent_drafted: false,
          created_at: "2026-09-20T09:00:00Z",
          updated_at: "2026-09-20T09:05:00Z",
        }),
      [DRAFT]: () => jsonResponse(drafted),
    });
    renderReply(true);

    await waitFor(() =>
      expect(messageText("Body")).toBe("Half written before lunch"),
    );
    await waitFor(() =>
      expect(draftButton().hasAttribute("disabled")).toBe(false),
    );
    expect(sent.filter((r) => r.key === DRAFT)).toHaveLength(0);
  });
});
