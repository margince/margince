/** @vitest-environment happy-dom */
import { QueryClient } from "@tanstack/react-query";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { messageText } from "../../design-system/richtext-testing";
import { draftButton } from "../../testing/composedraft.testkit";
import { ComposeModal, RelinkModal } from "./compose";
import {
  activity202,
  DISCLOSURE,
  emptyResponse,
  fillSendableForm,
  jsonResponse,
  PROVISIONAL_VOICE_PROFILE,
  problemResponse,
  render,
  stubRoutes,
  WHY_ASK,
} from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("RelinkModal", () => {
  it("relinks the search-picked target and closes on 200", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [{ type: "deal", id: "d-9", title: "Acme renewal" }],
          page: { has_more: false },
        }),
      "POST /activities/act-1/relink": () => jsonResponse(activity202),
    });
    const invalidate = vi.spyOn(QueryClient.prototype, "invalidateQueries");
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        entityType="contact"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "Acme");
    const candidate = await screen.findByRole("button", {
      name: "Acme renewal",
    });
    await userEvent.click(candidate);
    await userEvent.click(screen.getByRole("button", { name: "Relink" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const relink = sent.find((r) => r.key === "POST /activities/act-1/relink");
    expect(relink?.body).toEqual({
      entity_type: "deal",
      entity_id: "d-9",
      replace_existing_of_type: false,
    });
    // Relink is idempotency-keyed (its no-dup-on-replay contract).
    expect(relink?.headers.get("Idempotency-Key")).toBeTruthy();
    // AND it carries the version this reader saw. The key and the precondition
    // are one header slot, so a regression that wrote them separately would
    // send whichever came last — the key alone, silently unconditioned.
    expect(relink?.headers.get("If-Match")).toBe("4");
    // An email drawer still open behind the dialog reads its filing again.
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["email-presentation", "act-1"],
    });
  });

  // A copy read without a version cannot say what it is changing, so the relink
  // is refused BY NAME rather than through requireVersion's bare throw — which
  // reached the reader as the generic "something went wrong", on the very error
  // path the write leans on to say it did not go through.
  it("refuses in words when the activity was read without a version", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [{ type: "deal", id: "d-9", title: "Acme renewal" }],
          page: { has_more: false },
        }),
      "POST /activities/act-1/relink": () => jsonResponse(activity202),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={null}
        entityType="contact"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "Acme");
    await userEvent.click(
      await screen.findByRole("button", { name: "Acme renewal" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Relink" }));

    expect(await screen.findByText(/loaded without a version/i)).toBeTruthy();
    // And nothing was sent: a refusal that still wrote would be the
    // last-write-wins this whole change is about.
    expect(sent.find((r) => r.key === "POST /activities/act-1/relink")).toBe(
      undefined,
    );
    expect(onClose).not.toHaveBeenCalled();
  });

  it("sends replace_existing_of_type when the move toggle is on", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [{ type: "company", id: "o-2", title: "Globex" }],
          page: { has_more: false },
        }),
      "POST /activities/act-1/relink": () => jsonResponse(activity202),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        entityType="deal"
        entityId="d-1"
        open
        onClose={onClose}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "Globex");
    await userEvent.click(
      await screen.findByRole("button", { name: "Globex" }),
    );
    await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(screen.getByRole("button", { name: "Relink" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const relink = sent.find((r) => r.key === "POST /activities/act-1/relink");
    expect(relink?.body).toEqual({
      entity_type: "company",
      entity_id: "o-2",
      replace_existing_of_type: true,
    });
    expect(relink?.headers.get("If-Match")).toBe("4");
  });

  it("moves the whole conversation through relinkThread when asked", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [{ type: "project", id: "pr-4", title: "ERP rollout" }],
          page: { has_more: false },
        }),
      "POST /activities/relink-thread": () => jsonResponse({ relinked: 3 }),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        threadKey="thread:abc"
        entityType="contact"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "ERP");
    await userEvent.click(
      await screen.findByRole("button", { name: "ERP rollout" }),
    );
    await userEvent.click(
      screen.getByRole("checkbox", {
        name: "Move rest of thread",
      }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Relink" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const thread = sent.find((r) => r.key === "POST /activities/relink-thread");
    expect(thread?.body).toEqual({
      activity_id: "act-1",
      entity_type: "project",
      entity_id: "pr-4",
      replace_existing_of_type: false,
    });
    expect(thread?.headers.get("Idempotency-Key")).toBeTruthy();
    // And NO precondition: a thread moves many activities, and one version
    // cannot condition them — the server refuses a pinned batch by name.
    expect(thread?.headers.get("If-Match")).toBeNull();
    // The single-activity route is not called on the way.
    expect(sent.find((r) => r.key === "POST /activities/act-1/relink")).toBe(
      undefined,
    );
  });

  it("offers the conversation toggle only when the activity has a thread", () => {
    stubRoutes({});
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        entityType="contact"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );
    expect(
      screen.queryByRole("checkbox", {
        name: "Move rest of thread",
      }),
    ).toBeNull();
  });

  it("drops activity results — relink has no activity target", async () => {
    stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [
            { type: "activity", id: "a-x", title: "Some email" },
            { type: "contact", id: "pp-1", title: "Jane Doe" },
          ],
          page: { has_more: false },
        }),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        entityType="deal"
        entityId="d-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "e");
    expect(
      await screen.findByRole("button", { name: "Jane Doe" }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Some email" })).toBeNull();
  });
});

describe("ComposeModal", () => {
  it("fills To/Subject/Body from the AI draft", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          to: ["buyer@acme.test"],
        }),
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

    await userEvent.click(draftButton());

    expect(await screen.findByDisplayValue("Re: Q3 numbers")).toBeTruthy();
    await waitFor(() =>
      expect(messageText("Body")).toBe("Thanks for the note."),
    );
    // EmailDraft.to prefills the recipient chips.
    expect(screen.getByText("buyer@acme.test")).toBeTruthy();
  });

  // A model-produced draft reaching the rep without the provenance notice
  // presents as one a colleague wrote, so these three cases fix the banner's
  // presence, its verbatim text, and its absence on human-written text.
  // Removing the banner from the composer fails all three.
  it("discloses a model-produced draft, rendering the server's line verbatim", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: "Drafted with AI assistance. Review before sending.",
          draft_ref: null,
        }),
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

    await userEvent.click(draftButton());

    await screen.findByRole("heading", { name: "AI-assisted draft" });
    expect(
      screen.getByText("Drafted with AI assistance. Review before sending."),
    ).toBeTruthy();
  });

  it("still discloses when the server sends no disclosure line", async () => {
    // ai_disclosure is contract-guaranteed alongside ai_generated, but a
    // client that trusts that would drop the disclosure entirely against an
    // older or misbehaving server. Absence of the line is not absence of the
    // obligation, so the composer carries its own wording.
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: null,
          draft_ref: null,
        }),
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

    await userEvent.click(draftButton());

    await screen.findByRole("heading", { name: "AI-assisted draft" });
    expect(screen.getByText(/This draft was written by AI/i)).toBeTruthy();
  });

  it("discloses nothing when no model produced the draft", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: false,
          ai_disclosure: null,
          draft_ref: null,
        }),
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

    await userEvent.click(draftButton());

    // The fill proves the draft landed, so the missing banner is the
    // disclosure being conditional rather than the response never arriving.
    expect(await screen.findByDisplayValue("Re: Q3 numbers")).toBeTruthy();
    expect(screen.queryByRole("heading", DISCLOSURE)).toBeNull();
  });

  it("names the voice version that styled the draft and flags a provisional profile", async () => {
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: "Drafted with AI assistance. Review before sending.",
          voice_profile_version: 3,
          draft_ref: "vd-1",
        }),
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

    await userEvent.click(draftButton());

    expect(
      await screen.findByText("Built from your writing samples · v3"),
    ).toBeTruthy();
    expect(screen.getByText("Provisional voice")).toBeTruthy();
  });

  it("warns when the sender's voice could not be loaded for the draft", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: "Drafted with AI assistance. Review before sending.",
          voice_profile_version: null,
          draft_ref: null,
          voice_degraded: true,
        }),
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

    await userEvent.click(draftButton());

    expect(await screen.findByText(/does not use your voice/)).toBeTruthy();
  });

  it("stays quiet about the voice when a draft simply has no profile behind it", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: "Drafted with AI assistance. Review before sending.",
          voice_profile_version: null,
          draft_ref: null,
          voice_degraded: false,
        }),
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

    await userEvent.click(draftButton());

    expect(await screen.findByDisplayValue("Re: Q3 numbers")).toBeTruthy();
    expect(screen.queryByText(/does not use your voice/)).toBeNull();
  });

  it("flags nothing provisional when the profile is past that band", async () => {
    stubRoutes({
      "GET /voice-profiles": () =>
        jsonResponse({
          ...PROVISIONAL_VOICE_PROFILE,
          data: [
            { ...PROVISIONAL_VOICE_PROFILE.data[0], maturity: "building" },
          ],
        }),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: "Drafted with AI assistance. Review before sending.",
          voice_profile_version: 3,
          draft_ref: "vd-1",
        }),
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

    await userEvent.click(draftButton());

    expect(
      await screen.findByText("Built from your writing samples · v3"),
    ).toBeTruthy();
    expect(screen.queryByText("Provisional voice")).toBeNull();
  });

  it("shows an unavailable note on a 501 draft, keeping the form usable", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () => emptyResponse(501),
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

    await userEvent.click(draftButton());

    expect(await screen.findByText(/AI drafting is unavailable/i)).toBeTruthy();
    // Manual composing still works — Send is present.
    expect(screen.getByRole("button", { name: "Send" })).toBeTruthy();
  });

  // Send stays live with fields outstanding, and answers the press by naming
  // them. Grey, it refused and said nothing about what for, which left the
  // reader comparing the form against a control that would not talk to them.
  it("names every field it is waiting for, and sends nothing", async () => {
    const sent = stubRoutes();
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

    const send = screen.getByRole("button", { name: "Send" });
    expect(send.hasAttribute("disabled")).toBe(false);

    // Nothing is said before the reader has asked to send: a form reporting
    // what is missing while they are still typing is scolding them.
    expect(screen.queryByText("Enter a subject.")).toBeNull();

    await userEvent.click(send);

    expect(screen.getByText("Add at least one recipient.")).toBeTruthy();
    expect(screen.getByText("Enter a subject.")).toBeTruthy();
    expect(screen.getByText("Enter a message before sending.")).toBeTruthy();
    expect(screen.getByText("Select the reason for contact.")).toBeTruthy();
    expect(
      sent.filter((call) => call.key.startsWith("POST /activities")),
    ).toHaveLength(0);

    // And they stop being said as the fields are answered.
    await fillSendableForm();
    expect(screen.queryByText("Enter a subject.")).toBeNull();
  });

  it("sends the edited email with no approval token or idempotency key", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    // Nothing was drafted, so no voice-learning outcome may be inferred: the
    // exact-match assertion is what keeps `draft_ref` off an independently
    // composed send rather than riding along as null.
    expect(req?.body).toEqual({
      subject: "Hi there",
      body: "Body content",
      // Both renderings of one message. The plain part is what every gate
      // reads and what a text client receives; the markup rides beside it as
      // the alternative, never instead of it.
      html_body: "<p>Body content</p>",
      to: ["a@x.com"],
      communication_context: "requested_followup",
    });
    // ADR-0055: the human click is the approval — neither header rides along.
    expect(req?.headers.get("X-Approval-Token")).toBeNull();
    expect(req?.headers.get("Idempotency-Key")).toBeNull();
  });

  it("surfaces the default-deny consent gate on 409 without closing", async () => {
    const onClose = vi.fn();
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          {
            code: "consent_not_granted",
            detail: "suppressed",
            title: "Conflict",
          },
          409,
        ),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        contactId="p-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/has not granted consent/i)).toBeTruthy();
    // The gate points at the consent surface, and the modal stays open.
    expect(screen.getByRole("link", { name: "Review consent" })).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows a sending-unavailable note on a 501 send, not an error", async () => {
    const onClose = vi.fn();
    stubRoutes({
      "POST /activities/act-1/send-email": () => emptyResponse(501),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/Sending is unavailable/i)).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("does not report a send on a bodiless non-2xx (gateway 5xx)", async () => {
    const onClose = vi.fn();
    // openapi-fetch yields a falsy error for a bodiless response; success must
    // be the real 202, so an empty 503 stays an error and keeps the modal open
    // rather than falsely confirming an irreversible send.
    stubRoutes({
      "POST /activities/act-1/send-email": () => emptyResponse(503),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/The request failed/i)).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows a draft error on a bodiless non-2xx without crashing the fill", async () => {
    // The old !error success path would fabricate an undefined draft and crash
    // onSuccess reading its fields; a bodiless 502 must surface an error only.
    //
    // On THIS route the error says what happened: a draft runs for tens of
    // seconds, so a proxy giving up on one leaves work in flight, and a retry
    // is a second model call rather than a repeat. Every other route keeps the
    // generic sentence — see api/client.test.ts.
    stubRoutes({
      "POST /activities/act-1/draft-email": () => emptyResponse(502),
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
    await userEvent.click(draftButton());

    expect(
      await screen.findByText(/did not finish the request in time/i),
    ).toBeTruthy();
    // And it warns before a retry, because the first call may still be running.
    expect(screen.getByText(/may still be processing it/i)).toBeTruthy();
  });
});
