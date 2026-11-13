/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { REWRITE_MAX_CHARACTERS } from "../../lib/rewrites";
import { draftButton } from "../../testing/composedraft.testkit";
import { ComposeModal } from "./compose";
import {
  DISCLOSURE,
  emptyResponse,
  jsonResponse,
  PROVISIONAL_VOICE_PROFILE,
  REWRITE_ROUTE,
  render,
  rewrittenBody,
  stubRoutes,
  WHY_ASK,
} from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const DRAFT_ROUTE = "POST /activities/act-1/draft-email";

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

// The draft route answers the first draft; every verb press goes down the one
// rewrite path and answers `rewritten`.
async function openReply(
  first: string,
  rewritten: string | (() => Response) = "Polished.",
) {
  const sent = stubRoutes({
    "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
    [DRAFT_ROUTE]: draftsInTurn(first),
    [REWRITE_ROUTE]:
      typeof rewritten === "string"
        ? () => rewrittenBody(rewritten)
        : rewritten,
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
  return () => sent.filter((r) => r.key === REWRITE_ROUTE);
}

const polish = () => screen.getByRole("button", { name: "Polish" });
const undo = () => screen.queryByRole("button", { name: "Undo" });
const undoButton = () => screen.getByRole("button", { name: "Undo" });

describe("ComposeModal rewrites over the rep's words", () => {
  it("keeps the rewrites after the rep edits a draft, and revises the edit", async () => {
    const calls = await openReply("Draft A body.", "Polished.");
    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    writeMessage("Body", "Draft A body. And my own line.");

    await userEvent.click(polish());

    await waitFor(() => expect(messageText("Body")).toBe("Polished."));
    expect(calls()[0]?.body).toEqual({
      envelope: "email",
      verb: "polish",
      body: "Draft A body. And my own line.",
      language: "en",
      about: { entity_type: "activity", entity_id: "act-1" },
    });
  });

  it("polishes a body the rep wrote, discloses it, and an undo restores the rep's words", async () => {
    const calls = await openReply("Unused.", "Polished words.");
    const own = "hi anna, can we talk price next week";
    writeMessage("Body", own);
    expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull();

    await userEvent.click(polish());

    await waitFor(() => expect(messageText("Body")).toBe("Polished words."));
    expect(calls()[0]?.body).toMatchObject({ verb: "polish", body: own });
    expect(screen.getByRole("heading", DISCLOSURE)).toBeTruthy();

    await userEvent.click(undoButton());

    await waitFor(() => expect(messageText("Body")).toBe(own));
    expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull();
    expect(undo()).toBeNull();
  });

  it("offers Undo after every verb, back to the exact words before it", async () => {
    await openReply("Draft A body.", "Shorter words.");
    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    expect(undo()).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Shorter" }));
    await waitFor(() => expect(messageText("Body")).toBe("Shorter words."));
    await userEvent.click(undoButton());

    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    // The model's own draft is back, and its disclosure with it.
    expect(screen.getByRole("heading", DISCLOSURE)).toBeTruthy();
  });

  it("withdraws Undo once the rep edits the rewritten words", async () => {
    await openReply("Unused.", "Polished words.");
    writeMessage("Body", "hi anna, price next week");
    await userEvent.click(polish());
    await waitFor(() => expect(messageText("Body")).toBe("Polished words."));
    expect(undo()).not.toBeNull();

    writeMessage("Body", "Polished words. And my own line.");

    await waitFor(() => expect(undo()).toBeNull());
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

  it("bars the rewrites past the rewrite path's own limit, counted in characters, not UTF-16 units", async () => {
    const calls = await openReply("Unused.");
    writeMessage("Body", "😀".repeat(REWRITE_MAX_CHARACTERS));
    expect(polish()).toHaveProperty("disabled", false);

    writeMessage("Body", "😀".repeat(REWRITE_MAX_CHARACTERS + 1));
    await waitFor(() => expect(polish()).toHaveProperty("disabled", true));
    expect(screen.getByText("This mail is too long to rewrite.")).toBeTruthy();
    expect(calls()).toHaveLength(0);
  });

  it("says a body the server finds too long is too long, not that the call failed", async () => {
    await openReply("Unused.", () =>
      jsonResponse(
        {
          status: 422,
          code: "validation_error",
          details: {
            errors: [
              { field: "body", code: "too_long", message: "x", max: 12288 },
            ],
          },
        },
        422,
      ),
    );
    writeMessage("Body", "hi anna, price next week");

    await userEvent.click(polish());

    expect(
      await screen.findByText("This mail is too long to rewrite."),
    ).toBeTruthy();
    expect(messageText("Body")).toBe("hi anna, price next week");
  });
});
