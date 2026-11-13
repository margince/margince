/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { messageText } from "../../design-system/richtext-testing";
import { draftButton, purposeField } from "../../testing/composedraft.testkit";
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

  it("keeps the purpose it drafted to on screen once the draft stands", async () => {
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      [DRAFT]: () => jsonResponse(drafted),
    });
    renderReply(true);

    await waitFor(() =>
      expect(messageText("Body")).toBe("Tuesday works for us."),
    );
    expect(purposeField()).toHaveProperty(
      "value",
      "reply to their last message",
    );
    expect(screen.queryByRole("button", { name: "Redraft" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Draft / })).toBeNull();
  });

  it("redrafts once under the purpose the rep corrects", async () => {
    let call = 0;
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      [DRAFT]: () => {
        call += 1;
        return jsonResponse(
          call === 1
            ? drafted
            : { ...drafted, body: "Here is the offer.", draft_ref: "vd-b" },
        );
      },
    });
    renderReply(true);
    await waitFor(() =>
      expect(messageText("Body")).toBe("Tuesday works for us."),
    );

    await userEvent.clear(purposeField());
    await userEvent.type(purposeField(), "send the offer");
    await userEvent.click(screen.getByRole("button", { name: "Redraft" }));

    await waitFor(() => expect(messageText("Body")).toBe("Here is the offer."));
    const asks = sent.filter((r) => r.key === DRAFT);
    expect(asks).toHaveLength(2);
    expect(asks[1]?.body).toMatchObject({ intent: "send the offer" });
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
    expect(purposeField()).toHaveProperty(
      "value",
      "reply to their last message",
    );
    expect(screen.queryByRole("button", { name: "Redraft" })).toBeNull();
  });

  it("waits for a press when it cannot tell whether a draft is saved", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "GET /mail-drafts": () => jsonResponse({ code: "internal" }, 500),
      [DRAFT]: () => jsonResponse(drafted),
    });
    renderReply(true);

    await waitFor(() =>
      expect(sent.some((r) => r.key === "GET /mail-drafts")).toBe(true),
    );
    await waitFor(() =>
      expect(draftButton().hasAttribute("disabled")).toBe(false),
    );
    expect(sent.filter((r) => r.key === DRAFT)).toHaveLength(0);
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
