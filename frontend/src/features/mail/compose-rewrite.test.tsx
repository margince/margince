/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { draftButton } from "../../testing/composedraft.testkit";
import { ComposeModal } from "./compose";
import {
  DISCLOSURE,
  emptyResponse,
  jsonResponse,
  PROVISIONAL_VOICE_PROFILE,
  render,
  stubRoutes,
  WHY_ASK,
} from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const DRAFT_ROUTE = "POST /activities/act-1/draft-email";
const POLISH_ASK =
  "Improve the wording and flow. Keep the meaning, the facts and the length.";

function voiceDraft(ref: string, body: string) {
  return {
    subject: "Re: Q3",
    body,
    to: ["buyer@acme.test"],
    ai_generated: true,
    ai_disclosure: "Drafted with AI assistance. Review before sending.",
    voice_profile_version: 3,
    draft_ref: ref,
  };
}

function draftsInTurn(...bodies: string[]) {
  let call = 0;
  return () => {
    const body = bodies[Math.min(call, bodies.length - 1)] ?? "";
    call += 1;
    return jsonResponse(voiceDraft(`vd-${call}`, body));
  };
}

async function openReply(...bodies: string[]) {
  const sent = stubRoutes({
    "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
    [DRAFT_ROUTE]: draftsInTurn(...bodies),
    "PUT /mail-drafts": () => emptyResponse(204),
  });
  render(
    <ComposeModal
      activityId="act-1"
      entityType="contact"
      entityId="p-1"
      open
      onClose={vi.fn()}
    />,
  );
  await screen.findByRole("combobox", { name: WHY_ASK });
  return () => sent.filter((r) => r.key === DRAFT_ROUTE);
}

const polish = () => screen.getByRole("button", { name: "Polish" });

describe("ComposeModal rewrites over the rep's words", () => {
  it("keeps the rewrites after the rep edits a draft, and revises the edit", async () => {
    const calls = await openReply("Draft A body.", "Polished.");
    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    writeMessage("Body", "Draft A body. And my own line.");

    await userEvent.click(polish());

    await waitFor(() => expect(messageText("Body")).toBe("Polished."));
    expect(calls()[1]?.body).toMatchObject({
      intent: POLISH_ASK,
      rewrite_of: "Draft A body. And my own line.",
    });
  });

  it("polishes a body the rep wrote, discloses it, and an undo restores the rep's words", async () => {
    const calls = await openReply("Polished words.");
    const own = "hi anna, can we talk price next week";
    writeMessage("Body", own);
    expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull();

    await userEvent.click(polish());

    await waitFor(() => expect(messageText("Body")).toBe("Polished words."));
    expect(calls()[0]?.body).toMatchObject({
      intent: POLISH_ASK,
      rewrite_of: own,
    });
    expect(screen.getByRole("heading", DISCLOSURE)).toBeTruthy();

    writeMessage("Body", own);
    await waitFor(() =>
      expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull(),
    );
    expect(messageText("Body")).toBe(own);
  });

  it("offers no rewrite over an empty body", async () => {
    await openReply("Unused.");
    expect(screen.queryByRole("button", { name: "Polish" })).toBeNull();
  });

  it("bars the rewrites over a formatted body and says why", async () => {
    const calls = await openReply("Unused.");
    writeMessage("Body", "<strong>Price</strong> next week");

    expect(polish()).toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: "Shorter" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(
      screen.getByText(
        "Rewrites work on plain text. Remove the formatting and links to use them.",
      ),
    ).toBeTruthy();
    expect(calls()).toHaveLength(0);
  });
});
