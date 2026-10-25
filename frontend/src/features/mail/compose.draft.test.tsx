/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  messageBox,
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { draftButton } from "../../testing/composedraft.testkit";
import { ComposeModal } from "./compose";
import {
  activity202,
  DISCLOSURE,
  emptyResponse,
  fillSendableForm,
  jsonResponse,
  PROVISIONAL_VOICE_PROFILE,
  pickWhy,
  problemResponse,
  render,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
} from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// The 422 body httperr.Validation emits. Every validation problem carries the
// same top-level `code`. The rule that fired is named only in details.errors[],
// beside the field it was asserted about. A stub that hoisted the specific code
// to the top level would prove the screen reads a shape the server never sends.
function validationProblem(field: string, code: string, message: string) {
  return {
    code: "validation_error",
    title: "Unprocessable Entity",
    detail: message,
    details: { errors: [{ field, code, message }] },
  };
}

// rejectVoiceDraft answers the owner's updated learning aggregate; the composer
// only needs the call to have succeeded.
const LEARNING_SUMMARY = {
  drafted: 4,
  accepted: 1,
  edited_sent: 2,
  rejected: 1,
  qualifying_source_count: 0,
  qualifying_words: 0,
  transformations: [],
};

// A voice-styled draft, served with the reference the server keys a learning
// outcome against and the profile version that styled it.
function voiceDraft(
  ref: string,
  subject: string,
  body: string,
  voiceVersion = 3,
) {
  return {
    subject,
    body,
    to: ["buyer@acme.test"],
    ai_generated: true,
    ai_disclosure: "Drafted with AI assistance. Review before sending.",
    voice_profile_version: voiceVersion,
    draft_ref: ref,
  };
}

// Serves a different draft per call, so a test can re-draft and see which of
// the two references the composer is still holding.
function draftsInTurn(...drafts: object[]) {
  let call = 0;
  return () => {
    const drafted = drafts[Math.min(call, drafts.length - 1)];
    call += 1;
    return jsonResponse(drafted);
  };
}

const KEPT_NOTICE = "Your edits were kept. Draft again when ready.";

// Once a draft stands, the ask is folded behind its own control on the line.
async function askAgain() {
  await userEvent.click(screen.getByRole("button", { name: "Change the ask" }));
  await userEvent.click(draftButton());
}

function renderComposer(onClose = vi.fn()) {
  render(
    <ComposeModal
      activityId="act-1"
      entityType="contact"
      entityId="p-1"
      open
      onClose={onClose}
    />,
  );
  return onClose;
}

// The send-time binding to the voice draft: `draft_ref` is what makes the
// server's accepted/edited_sent verdict about the rep's own words.
describe("ComposeModal draft binding", () => {
  it("sends the reference of the draft whose text it is sending", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await pickWhy(WHY_LABEL.requestedFollowup);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({
      body: "Draft A body.",
      draft_ref: "vd-a",
    });
  });

  it("drafts again with a changed ask", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        voiceDraft("vd-b", "Re: Q3", "Draft B body."),
      ),
      // Typing the ask is long enough for the composer to keep a draft.
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await userEvent.click(
      screen.getByRole("button", { name: "Change the ask" }),
    );
    const ask = screen.getByRole("textbox", { name: "Purpose of the reply" });
    await userEvent.clear(ask);
    await userEvent.type(ask, "Offer Tuesday instead");
    await userEvent.click(draftButton());

    await waitFor(() =>
      expect(
        sent.filter((r) => r.key === "POST /activities/act-1/draft-email")[1]
          ?.body,
      ).toMatchObject({ intent: "Offer Tuesday instead" }),
    );
  });

  it("puts the second reply draft on screen and sends the first as rewrite_of", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        voiceDraft("vd-b", "Re: Q3", "Draft B body."),
      ),
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await askAgain();

    await waitFor(() => expect(messageText("Body")).toBe("Draft B body."));
    const calls = sent.filter(
      (r) => r.key === "POST /activities/act-1/draft-email",
    );
    expect(calls[0]?.body).toEqual({});
    expect(calls[1]?.body).toMatchObject({ rewrite_of: "Draft A body." });
    expect(screen.queryByText(KEPT_NOTICE)).toBeNull();
  });

  it("keeps the rep's edit on a reply, sends no rewrite_of and says so", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        voiceDraft("vd-b", "Re: Q3", "Draft B body."),
      ),
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    writeMessage("Body", "Draft A body. And my own line.");
    await askAgain();

    expect(await screen.findByText(KEPT_NOTICE)).toBeTruthy();
    expect(messageText("Body")).toBe("Draft A body. And my own line.");
    const calls = sent.filter(
      (r) => r.key === "POST /activities/act-1/draft-email",
    );
    expect(calls[1]?.body).not.toHaveProperty("rewrite_of");
  });

  it("replaces the body on a reply when the rep presses a rewrite verb", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        voiceDraft("vd-b", "Re: Q3", "Short."),
      ),
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await userEvent.click(screen.getByRole("button", { name: "Shorter" }));

    await waitFor(() => expect(messageText("Body")).toBe("Short."));
    const calls = sent.filter(
      (r) => r.key === "POST /activities/act-1/draft-email",
    );
    expect(calls[1]?.body).toMatchObject({ rewrite_of: "Draft A body." });
  });

  it("keeps the earlier reference when a re-draft leaves the typed body alone", async () => {
    // The fill rule never clobbers text the rep has touched, so the second
    // draft's words are discarded. Adopting its reference anyway would submit
    // draft B's identity with draft A's text plus the rep's edit. That is an
    // edited_sent whose "edit" is a draft the rep never saw.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        voiceDraft("vd-b", "Re: Q3 again", "Draft B body."),
      ),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    writeMessage("Body", "Draft A body. And my own line.");
    await askAgain();
    await waitFor(() =>
      expect(
        sent.filter((r) => r.key === "POST /activities/act-1/draft-email")
          .length,
      ).toBe(2),
    );

    await pickWhy(WHY_LABEL.requestedFollowup);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({
      body: "Draft A body. And my own line.",
      draft_ref: "vd-a",
    });
  });

  it("drops the reference once the body it named is cleared", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    // Emptied, then written afresh: two acts, and the first is the one under
    // test. Replacing the words in one stroke keeps the reference, because
    // the server compares it against the final body and records an edited send.
    // An empty box is the stronger case, where there is no longer a draft on
    // screen for a reference to name at all.
    writeMessage("Body", "");
    writeMessage("Body", "Written from scratch.");
    await pickWhy(WHY_LABEL.requestedFollowup);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).not.toHaveProperty("draft_ref");
  });

  it("records exactly one rejection when the rep discards the draft", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        jsonResponse(LEARNING_SUMMARY),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );

    await waitFor(() => expect(messageText("Body")).not.toBe("Draft A body."));
    const rejections = sent.filter(
      (r) => r.key === "POST /voice-profiles/vp-1/draft-rejections",
    );
    expect(rejections).toHaveLength(1);
    expect(rejections[0].body).toEqual({ draft_ref: "vd-a" });
  });

  it("bars a send while a rejection of the same draft is in flight", async () => {
    // A rejection and a send are contradictory verdicts on one draft, and the
    // signal keeps whichever lands first. A send that slipped through here
    // would leave the durable learning record saying "rejected" for a message
    // that went out. The explicit judgment exists to prevent that failure.
    let landRejection = (): void => {};
    const rejectionInFlight = new Promise<void>((resolve) => {
      landRejection = resolve;
    });
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        rejectionInFlight.then(() => jsonResponse(LEARNING_SUMMARY)),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await pickWhy(WHY_LABEL.requestedFollowup);
    // The form is sendable before the judgment starts, so the refusal below is
    // the rejection holding the draft rather than an unmet precondition.
    expect(
      screen.getByRole("button", { name: "Send" }).hasAttribute("disabled"),
    ).toBe(false);

    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Send" }).hasAttribute("disabled"),
      ).toBe(true),
    );
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(
      sent.some((r) => r.key === "POST /activities/act-1/send-email"),
    ).toBe(false);

    landRejection();
    await waitFor(() => expect(messageText("Body")).not.toBe("Draft A body."));
    expect(
      sent.some((r) => r.key === "POST /activities/act-1/send-email"),
    ).toBe(false);
  });

  it("hands the reference back to the send when the rejection fails", async () => {
    // A rejection that never landed left the signal open, so the draft on
    // screen is still the one it named. The send that follows must still be
    // able to report what the rep did with those words.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        problemResponse(
          { code: "internal", title: "Server Error", detail: "boom" },
          500,
        ),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );
    expect(await screen.findByText(/boom/i)).toBeTruthy();

    await pickWhy(WHY_LABEL.requestedFollowup);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({ draft_ref: "vd-a" });
  });

  it("keeps the draft when a bodiless gateway failure refuses the rejection", async () => {
    // openapi-fetch reports a falsy `error` and no `data` for a bodiless
    // non-2xx. So a rejection that never reached the server looks like one that
    // succeeded. Clearing the composer on it would tell the rep their verdict
    // was recorded while the signal is still open. The next send would then be
    // classified against a draft the rep believes they discarded.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () => emptyResponse(502),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );

    expect(await screen.findByText("The request failed. Retry.")).toBeTruthy();
    expect(messageText("Body")).toBe("Draft A body.");

    await pickWhy(WHY_LABEL.requestedFollowup);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({ draft_ref: "vd-a" });
  });

  it("announces a failed rejection rather than only colouring it", async () => {
    // The line appears without any navigation, so a rep who cannot see it is
    // otherwise never told the judgment did not land.
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        problemResponse(
          { code: "internal", title: "Server Error", detail: "boom" },
          500,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );

    const announced = await screen.findAllByRole("alert");
    expect(announced.map((node) => node.textContent)).toContain("boom");
  });

  it("announces a failed draft rather than only colouring it", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        problemResponse(
          { code: "internal", title: "Server Error", detail: "no model" },
          500,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());

    const announced = await screen.findAllByRole("alert");
    expect(announced.map((node) => node.textContent)).toContain("no model");
  });

  it("freezes the drafted text while the rejection is in flight", async () => {
    // A failed rejection hands its reference back to the surface. If the rep
    // could type meanwhile, that reference would return over words it does not
    // describe. A later send would file an outcome against a draft nobody
    // wrote. That is the same defect as adopting a reference whose body was
    // never applied.
    let landRejection = (): void => {};
    const rejectionInFlight = new Promise<void>((resolve) => {
      landRejection = resolve;
    });
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        rejectionInFlight.then(() => jsonResponse(LEARNING_SUMMARY)),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    const bodyField = messageBox("Body");
    const subjectField = screen.getByDisplayValue("Re: Q3");
    // The body is a contentEditable surface, so its refusal is stated in ARIA
    // rather than by the `disabled` attribute an input carries.
    expect(bodyField.getAttribute("aria-disabled")).toBeNull();

    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );
    await waitFor(() =>
      expect(bodyField.getAttribute("aria-disabled")).toBe("true"),
    );
    expect(subjectField.hasAttribute("disabled")).toBe(true);
    await userEvent.type(bodyField, " and mine");
    expect(messageText("Body")).toBe("Draft A body.");

    landRejection();
    await waitFor(() => expect(messageText("Body")).not.toBe("Draft A body."));
  });

  it("records no rejection when the composer is merely closed", async () => {
    // `rejected` is a judgment, not an accident of navigation. The reference is
    // deterministic and the drafted signal inserts once. So a rejection logged
    // on a close would stand in for the real outcome of an identical draft that
    // is later sent.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      // Closing keeps the words as a saved draft, and the window shuts once
      // the server has kept them.
      "PUT /mail-drafts": () =>
        jsonResponse({
          id: "md-1",
          anchor_type: "activity",
          anchor_id: "act-1",
          to: [],
          cc: [],
          bcc: [],
          subject: "Re: Q3",
          body: "Draft A body.",
          html_body: null,
          version: 1,
          agent_drafted: true,
          created_at: "2026-08-01T00:00:00Z",
          updated_at: "2026-08-01T00:00:00Z",
        }),
    });
    const onClose = renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await userEvent.click(screen.getByRole("button", { name: "Close" }));

    expect(onClose).toHaveBeenCalled();
    expect(sent.some((r) => r.key.includes("draft-rejections"))).toBe(false);
  });
});

// The provenance banner describes the words on screen. It rides on the
// condition that puts a served draft there, and leaves when they do. A banner
// that outlived its text would attribute a human's writing to a model, or
// credit it to a voice version that never touched it.
describe("ComposeModal draft provenance", () => {
  it("discloses nothing when a draft's words were discarded for the rep's own", async () => {
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    writeMessage("Body", "My own words.");
    await userEvent.click(draftButton());

    // Wait for draft completion before checking the discarded body's provenance.
    expect(await screen.findByDisplayValue("Re: Q3")).toBeTruthy();
    expect(messageText("Body")).toBe("My own words.");
    expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull();
  });

  it("keeps the applied draft's voice version when a re-draft is discarded", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body.", 3),
        voiceDraft("vd-b", "Re: Q3 again", "Draft B body.", 4),
      ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    writeMessage("Body", "Draft A body. And my own line.");
    await askAgain();
    await waitFor(() =>
      expect(
        sent.filter((r) => r.key === "POST /activities/act-1/draft-email")
          .length,
      ).toBe(2),
    );

    // v4's words were never applied, so v4 may not be named over v3's.
    expect(
      screen.getByText("Built from your writing samples · v3"),
    ).toBeTruthy();
    expect(
      screen.queryByText("Built from your writing samples · v4"),
    ).toBeNull();
  });

  it("drops the disclosure once the body it described is cleared", async () => {
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    expect(screen.getByRole("heading", DISCLOSURE)).toBeTruthy();

    writeMessage("Body", "");

    await waitFor(() =>
      expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull(),
    );
  });

  it("claims no voice maturity on a draft no voice profile styled", async () => {
    // Maturity is a corpus-word band, so it reads `provisional` while the
    // profile is still only collecting. Then drafting serves a plain model
    // draft and the served version is null. The provisional copy says a voice
    // already shapes the draft, which here nothing does.
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          ...voiceDraft("vd-a", "Re: Q3", "Draft A body."),
          voice_profile_version: null,
        }),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await userEvent.click(draftButton());

    await screen.findByRole("heading", { name: "AI-assisted draft" });
    expect(screen.queryByText("Provisional voice")).toBeNull();
  });
});

// The send pre-flight's two refusals. Each is a product state with a fix, so
// each must read as its own copy. It must not fall into the generic failure
// line that the server's detail string would otherwise land in.
describe("ComposeModal send refusals", () => {
  it("tells the rep to reconnect a capture-only mailbox, and where", async () => {
    const onClose = vi.fn();
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "from",
            "mailbox_not_send_capable",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer(onClose);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/without permission to send/i)).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Reconnect your mailbox" })
        .getAttribute("href"),
    ).toBe("#/settings/connections");
    // The refusal replaces the generic line rather than joining it.
    expect(screen.queryByText("opaque server wording")).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("states the one-send-per-recipient rule on a shared unsubscribe token", async () => {
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "recipients",
            "shared_unsubscribe_token",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(
      await screen.findByText(/goes to one recipient at a time/i),
    ).toBeTruthy();
    expect(screen.queryByText("opaque server wording")).toBeNull();
  });

  it("keeps the generic line for a refusal it has no copy for", async () => {
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "subject",
            "some_future_refusal",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/opaque server wording/i)).toBeTruthy();
  });

  it("does not read a refusal off the field the server did not assert it about", async () => {
    // The pointed copy answers a specific input. "Reconnect your mailbox" is
    // an answer about `from`, and would be the wrong instruction under a later
    // rule that refused some other field. So the pair is matched, not the code
    // alone, and an unrecognised pair keeps the server's own wording.
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "recipients",
            "mailbox_not_send_capable",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/opaque server wording/i)).toBeTruthy();
    expect(screen.queryByText(/without permission to send/i)).toBeNull();
  });

  it("warns about a second addressee before the send, not after it", async () => {
    // Every purpose but transactional renders one recipient's unsubscribe link,
    // so the server refuses a second addressee outright. Saying so after the
    // round trip is strictly worse than saying so while the rep can still fix it.
    const sent = stubRoutes();
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Cc and Bcc" }));
    await userEvent.type(screen.getByLabelText("Cc"), "second@x.com");
    await userEvent.tab();

    expect(screen.queryByText(/more than one recipient/i)).toBeNull();
    await pickWhy(WHY_LABEL.marketing);

    // The warning bar above the foot says what it is.
    expect(await screen.findByText(/more than one recipient/i)).toBeTruthy();
    // A warning, not a gate, and nothing was sent to earn it.
    expect(
      sent.some((r) => r.key === "POST /activities/act-1/send-email"),
    ).toBe(false);
  });

  it("does not warn about a lone addressee under the same purpose", async () => {
    stubRoutes();
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await pickWhy(WHY_LABEL.marketing);

    expect(screen.queryByText(/more than one recipient/i)).toBeNull();
  });
});
