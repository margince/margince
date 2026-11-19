/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  messageBox,
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import {
  pressDraft,
  purposeField,
  redraft,
} from "../../testing/composedraft.testkit";
import {
  activity202,
  DISCLOSURE,
  emptyResponse,
  installComposeClock,
  jsonResponse,
  PROVISIONAL_VOICE_PROFILE,
  pickWhy,
  pressSend,
  problemResponse,
  REWRITE_ROUTE,
  renderComposer,
  rewrittenBody,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
  waitForSendReady,
} from "./compose.testkit";

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

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

// Answers each draft call when the test says, so it can type while one is out.
function draftsOnCue() {
  const pending: ((response: Response) => void)[] = [];
  const asked = (call: number) =>
    waitFor(() => expect(pending[call]).toBeTypeOf("function"));
  return {
    route: () =>
      new Promise<Response>((resolve) => {
        pending.push(resolve);
      }),
    asked,
    answer: async (call: number, drafted: object) => {
      await asked(call);
      pending[call]?.(jsonResponse(drafted));
    },
  };
}

// The send-time binding to the voice draft: `draft_ref` is what makes the
// server's accepted/edited_sent verdict about the rep's own words.
describe("ComposeModal draft binding", () => {
  it("sends the reference of the draft whose text it is sending", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await pickWhy(user, WHY_LABEL.requestedFollowup);
    await pressSend(user);

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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    const ask = screen.getByRole("textbox", { name: "Purpose of the reply" });
    expect(screen.queryByRole("button", { name: "Redraft" })).toBeNull();
    await user.clear(ask);
    await user.type(ask, "Offer Tuesday instead");
    await user.click(screen.getByRole("button", { name: "Redraft" }));

    await waitFor(() => expect(messageText("Body")).toBe("Draft B body."));
    const calls = sent.filter(
      (r) => r.key === "POST /activities/act-1/draft-email",
    );
    expect(calls).toHaveLength(2);
    expect(calls[1]?.body).toMatchObject({ intent: "Offer Tuesday instead" });
    // A new draft is asked for under the purpose it stands on.
    expect(purposeField()).toHaveProperty("value", "Offer Tuesday instead");
    expect(screen.queryByRole("button", { name: "Redraft" })).toBeNull();
  });

  it("keeps Redraft beside a changed purpose that a rewrite never read", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        voiceDraft("vd-b", "Re: Q3", "Draft B body."),
      ),
      [REWRITE_ROUTE]: () => rewrittenBody("Short."),
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));

    await user.type(purposeField(), "Offer Tuesday");
    await user.click(screen.getByRole("button", { name: "Shorter" }));
    await waitFor(() => expect(messageText("Body")).toBe("Short."));
    await user.click(screen.getByRole("button", { name: "Redraft" }));

    await waitFor(() => expect(messageText("Body")).toBe("Draft B body."));
    const calls = sent.filter(
      (r) => r.key === "POST /activities/act-1/draft-email",
    );
    expect(calls).toHaveLength(2);
    expect(calls[1]?.body).toMatchObject({ intent: "Offer Tuesday" });
  });

  it("announces a failed Redraft and keeps it on offer", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    let call = 0;
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () => {
        call += 1;
        return call === 1
          ? jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body."))
          : problemResponse(
              { code: "internal", title: "Server Error", detail: "no model" },
              500,
            );
      },
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));

    await redraft(user);

    const announced = await screen.findAllByRole("alert");
    expect(announced.map((node) => node.textContent)).toContain("no model");
    expect(messageText("Body")).toBe("Draft A body.");
    expect(screen.getByRole("button", { name: "Redraft" })).toBeTruthy();
  });

  it("drops a Redraft answer once the purpose moves on, and says the edits were kept", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const drafts = draftsOnCue();
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": drafts.route,
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await pressDraft(user);
    await drafts.answer(0, voiceDraft("vd-a", "Re: Q3", "Draft A body."));
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));

    await redraft(user);
    await drafts.asked(1);
    await user.type(purposeField(), " on Tuesday");
    await drafts.answer(1, voiceDraft("vd-b", "Re: Q3", "Draft B body."));

    expect(await screen.findByText(KEPT_NOTICE)).toBeTruthy();
    expect(messageText("Body")).toBe("Draft A body.");
    expect(
      sent.filter((r) => r.key === "POST /activities/act-1/draft-email"),
    ).toHaveLength(2);
  });

  it("sends the subject on screen with a reply draft, so it can set the language", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        voiceDraft("vd-b", "Unsere nächsten Schritte", "Entwurf B."),
      ),
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);
    const subjectField = await screen.findByDisplayValue("Re: Q3");
    await user.clear(subjectField);
    await user.type(subjectField, "Unsere nächsten Schritte");
    await redraft(user);

    await waitFor(() =>
      expect(
        sent.filter((r) => r.key === "POST /activities/act-1/draft-email")[1]
          ?.body,
      ).toMatchObject({ subject: "Unsere nächsten Schritte" }),
    );
  });

  it("puts the second reply draft on screen and sends the first as rewrite_of", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await redraft(user);

    await waitFor(() => expect(messageText("Body")).toBe("Draft B body."));
    const calls = sent.filter(
      (r) => r.key === "POST /activities/act-1/draft-email",
    );
    expect(calls[0]?.body).toEqual({});
    expect(calls[1]?.body).toMatchObject({ rewrite_of: "Draft A body." });
    expect(screen.queryByText(KEPT_NOTICE)).toBeNull();
  });

  it("revises the rep's edit on a reply, sending it as rewrite_of", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    writeMessage("Body", "Draft A body. And my own line.");
    await redraft(user);

    await waitFor(() => expect(messageText("Body")).toBe("Draft B body."));
    const calls = sent.filter(
      (r) => r.key === "POST /activities/act-1/draft-email",
    );
    expect(calls[1]?.body).toMatchObject({
      rewrite_of: "Draft A body. And my own line.",
    });
    expect(screen.queryByText(KEPT_NOTICE)).toBeNull();
  });

  it("replaces the body on a reply when the rep presses a rewrite verb", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
      ),
      [REWRITE_ROUTE]: () => rewrittenBody("Short."),
      "PUT /mail-drafts": () => emptyResponse(204),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await user.click(screen.getByRole("button", { name: "Shorter" }));

    await waitFor(() => expect(messageText("Body")).toBe("Short."));
    expect(sent.find((r) => r.key === REWRITE_ROUTE)?.body).toMatchObject({
      envelope: "email",
      verb: "shorter",
      body: "Draft A body.",
      about: { entity_type: "activity", entity_id: "act-1" },
    });
  });

  it("keeps the earlier reference when a re-draft leaves the typed body alone", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    // A draft no model revised leaves the typed words on screen, so its words
    // are discarded. Adopting its reference anyway would submit draft B's
    // identity with draft A's text plus the rep's edit. That is an edited_sent
    // whose "edit" is a draft the rep never saw.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        {
          ...voiceDraft("vd-b", "Re: Q3 again", "Draft B body."),
          ai_generated: false,
        },
      ),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    writeMessage("Body", "Draft A body. And my own line.");
    await redraft(user);
    await waitFor(() =>
      expect(
        sent.filter((r) => r.key === "POST /activities/act-1/draft-email")
          .length,
      ).toBe(2),
    );

    await pickWhy(user, WHY_LABEL.requestedFollowup);
    await pressSend(user);

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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    // Emptied, then written afresh: two acts, and the first is the one under
    // test. Replacing the words in one stroke keeps the reference, because
    // the server compares it against the final body and records an edited send.
    // An empty box is the stronger case, where there is no longer a draft on
    // screen for a reference to name at all.
    writeMessage("Body", "");
    writeMessage("Body", "Written from scratch.");
    await pickWhy(user, WHY_LABEL.requestedFollowup);
    await pressSend(user);

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).not.toHaveProperty("draft_ref");
  });

  it("records exactly one rejection when the rep discards the draft", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        jsonResponse(LEARNING_SUMMARY),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await user.click(screen.getByRole("button", { name: "Discard draft" }));

    await waitFor(() => expect(messageText("Body")).not.toBe("Draft A body."));
    const rejections = sent.filter(
      (r) => r.key === "POST /voice-profiles/vp-1/draft-rejections",
    );
    expect(rejections).toHaveLength(1);
    expect(rejections[0].body).toEqual({ draft_ref: "vd-a" });
  });

  it("bars a send while a rejection of the same draft is in flight", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await pickWhy(user, WHY_LABEL.requestedFollowup);
    await waitForSendReady();
    // The form is sendable before the judgment starts, so the refusal below is
    // the rejection holding the draft rather than an unmet precondition.
    expect(
      screen.getByRole("button", { name: "Send" }).hasAttribute("disabled"),
    ).toBe(false);

    await user.click(screen.getByRole("button", { name: "Discard draft" }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Send" }).hasAttribute("disabled"),
      ).toBe(true),
    );
    await user.click(screen.getByRole("button", { name: "Send" }));
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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await user.click(screen.getByRole("button", { name: "Discard draft" }));
    expect(await screen.findByText(/boom/i)).toBeTruthy();

    await pickWhy(user, WHY_LABEL.requestedFollowup);
    await pressSend(user);

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({ draft_ref: "vd-a" });
  });

  it("keeps the draft when a bodiless gateway failure refuses the rejection", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await user.click(screen.getByRole("button", { name: "Discard draft" }));

    expect(await screen.findByText("The request failed. Retry.")).toBeTruthy();
    expect(messageText("Body")).toBe("Draft A body.");

    await pickWhy(user, WHY_LABEL.requestedFollowup);
    await pressSend(user);
    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({ draft_ref: "vd-a" });
  });

  it("announces a failed rejection rather than only colouring it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await user.click(screen.getByRole("button", { name: "Discard draft" }));

    const announced = await screen.findAllByRole("alert");
    expect(announced.map((node) => node.textContent)).toContain("boom");
  });

  it("announces a failed draft rather than only colouring it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        problemResponse(
          { code: "internal", title: "Server Error", detail: "no model" },
          500,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);

    const announced = await screen.findAllByRole("alert");
    expect(announced.map((node) => node.textContent)).toContain("no model");
  });

  it("freezes the drafted text while the rejection is in flight", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    const bodyField = messageBox("Body");
    const subjectField = screen.getByDisplayValue("Re: Q3");
    // The body is a contentEditable surface, so its refusal is stated in ARIA
    // rather than by the `disabled` attribute an input carries.
    expect(bodyField.getAttribute("aria-disabled")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Discard draft" }));
    await waitFor(() =>
      expect(bodyField.getAttribute("aria-disabled")).toBe("true"),
    );
    expect(subjectField.hasAttribute("disabled")).toBe(true);
    await user.type(bodyField, " and mine");
    expect(messageText("Body")).toBe("Draft A body.");

    landRejection();
    await waitFor(() => expect(messageText("Body")).not.toBe("Draft A body."));
  });

  it("records no rejection when the composer is merely closed", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await user.click(screen.getByRole("button", { name: "Close" }));

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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const drafts = draftsOnCue();
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": drafts.route,
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    writeMessage("Body", "My own words.");
    await pressDraft(user);
    await drafts.asked(0);
    writeMessage("Body", "My own words, and more.");
    await drafts.answer(0, voiceDraft("vd-a", "Re: Q3", "Draft A body."));

    expect(await screen.findByText(KEPT_NOTICE)).toBeTruthy();
    expect(messageText("Body")).toBe("My own words, and more.");
    expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull();
  });

  it("keeps the applied draft's voice version when a re-draft is discarded", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const drafts = draftsOnCue();
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": drafts.route,
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);
    await drafts.answer(0, voiceDraft("vd-a", "Re: Q3", "Draft A body.", 3));
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    await redraft(user);
    await drafts.asked(1);
    writeMessage("Body", "Draft A body. And my own line.");
    await drafts.answer(
      1,
      voiceDraft("vd-b", "Re: Q3 again", "Draft B body.", 4),
    );
    expect(await screen.findByText(KEPT_NOTICE)).toBeTruthy();

    // v4's words were never applied, so v4 may not be named over v3's.
    expect(
      screen.getByText("Built from your writing samples · v3"),
    ).toBeTruthy();
    expect(
      screen.queryByText("Built from your writing samples · v4"),
    ).toBeNull();
  });

  it("drops the disclosure once the body it described is cleared", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });

    await pressDraft(user);
    await waitFor(() => expect(messageText("Body")).toBe("Draft A body."));
    expect(screen.getByRole("heading", DISCLOSURE)).toBeTruthy();

    writeMessage("Body", "");

    await waitFor(() =>
      expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull(),
    );
  });

  it("claims no voice maturity on a draft no voice profile styled", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

    await pressDraft(user);

    await screen.findByRole("heading", { name: "AI-assisted draft" });
    expect(screen.queryByText("Provisional voice")).toBeNull();
  });
});
