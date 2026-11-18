// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ASYNC_UTIL_TIMEOUT_MS,
  SLOWEST_MEASURED_TEST_MS,
} from "../../../vitest.budget";
import { writeMessage } from "../../design-system/richtext-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { meFixture } from "../../testing/mefixture";
import { isPreviewDoor } from "../../testing/sendpermission.testkit";
import {
  activity202,
  fillSendableForm,
  jsonResponse,
  pressSend,
  problemResponse,
  render,
  type Sent,
  stubRoutes,
  WHY_ASK,
} from "./compose.testkit";
import { draftKey } from "./composesaveddraftcalls";
import { ComposeWindow } from "./composewindow";

// The email composer with the inline pilot switch on. Nothing is asked while
// the rep writes, and Send is the check. A refusal comes back as a review,
// drawn in place with one reason and Send anyway.

// The in-flight and refreshed-review tests each chain eight waits, past the
// default ceiling.
const EIGHT_WAITS_TEST_MS =
  ASYNC_UTIL_TIMEOUT_MS * 8 + SLOWEST_MEASURED_TEST_MS;

const INLINE_TEXT =
  "The send check still blocks this message. Send anyway sends only this message and grants no permission for later ones. Your name, your reason and the message are kept in a record that cannot be edited.";

const REVIEW = {
  id: "r-1",
  state: "needs_context",
  kind: "single",
  reason_code: "marketing_objection",
  assessment_revision: "rev-1",
  delivery_intent_id: "intent-1",
  refusals: [
    { address: "a@x.com", reason_code: "marketing_objection" },
    { address: "b@x.com", reason_code: "no_compatible_evidence" },
  ],
  warning: { version: "override-v2", text: "The modal's longer words." },
  warnings: [
    { version: "override-v2", text: "The modal's longer words." },
    { version: "inline-v1", text: INLINE_TEXT },
  ],
};

function refused(
  details: Record<string, unknown> = {},
  review: unknown = REVIEW,
) {
  return problemResponse(
    {
      code: "consent_not_granted",
      title: "Conflict",
      status: 409,
      details: {
        review_id: "r-1",
        available_actions: ["direct_send"],
        refusals: [],
        ...(review === null ? {} : { review }),
        ...details,
      },
    },
    409,
  );
}

const inlineMe = () =>
  jsonResponse({
    ...meFixture({ allow: { activity: ["read", "create", "update"] } }),
    inline_send_review: true,
  });

const count = (sent: Sent[], key: string) =>
  sent.filter((entry) => entry.key === key).length;
const previews = (sent: Sent[]) =>
  sent.filter((entry) => isPreviewDoor(entry.key)).length;

function Composer({ onClose = () => {} }: Readonly<{ onClose?: () => void }>) {
  const [open, setOpen] = useState(true);
  return (
    <ToastProvider>
      <ComposeWindow
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        open={open}
        onClose={() => {
          setOpen(false);
          onClose();
        }}
      />
      <ToastRegion />
    </ToastProvider>
  );
}

// Waits for the switch to be read: the reason-for-contact dial goes away.
async function openInline(sent: Sent[]) {
  render(<Composer />);
  await waitFor(() => expect(count(sent, "GET /me")).toBeGreaterThan(0));
  await waitFor(() =>
    expect(screen.queryByRole("combobox", { name: WHY_ASK })).toBeNull(),
  );
}

async function fillInline(user: UserEvent) {
  await user.type(screen.getByLabelText("To"), "a@x.com");
  await user.tab();
  await user.type(screen.getByLabelText("Subject"), "Your quote");
  writeMessage("Body", "The prices you asked for.");
}

// The callout's title; the detail behind the count lists it again.
const leadFinding = async () =>
  (
    await screen.findAllByText("a@x.com objected to marketing.")
  )[0] as HTMLElement;
const reasonField = () => screen.findByLabelText(/Reason for sending/);
const sendAnyway = () => screen.findByRole("button", { name: "Send anyway" });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the email composer with the switch off", () => {
  it("keeps the check while writing and the reason for contact", async () => {
    const sent = stubRoutes();
    render(<Composer />);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    expect(previews(sent)).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Send anyway" })).toBeNull();
  });
});

describe("the email composer with the switch on", () => {
  it("asks nothing while the rep opens, types and the draft saves", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({ "GET /me": inlineMe });
    await openInline(sent);
    await fillInline(user);
    await waitFor(() =>
      expect(count(sent, "PUT /mail-drafts")).toBeGreaterThan(0),
    );
    expect(previews(sent)).toBe(0);
    expect(count(sent, "POST /lawful-basis-confirmations")).toBe(0);
  });

  it("sends a clean message with one request and closes", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    render(<Composer onClose={onClose} />);
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: WHY_ASK })).toBeNull(),
    );
    await fillInline(user);
    await pressSend(user);
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(count(sent, "POST /emails")).toBe(1);
    const body = sent.find((entry) => entry.key === "POST /emails")?.body;
    expect(body).not.toHaveProperty("communication_context");
    expect(previews(sent)).toBe(0);
  });

  it("closes a scheduled send with the scheduled note", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": () => jsonResponse(activity202, 201),
    });
    render(<Composer />);
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: WHY_ASK })).toBeNull(),
    );
    await fillInline(user);
    await pressSend(user);
    expect(
      await screen.findByText("Email scheduled. It has not been sent yet."),
    ).toBeTruthy();
  });

  it("draws a refusal from the embedded review without asking again", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({ "GET /me": inlineMe, "POST /emails": refused });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);

    expect(await leadFinding()).toBeTruthy();
    expect(screen.getByText(INLINE_TEXT)).toBeTruthy();
    expect(screen.getByText("1 more finding")).toBeTruthy();
    expect(screen.getByText(", named in the warning")).toBeTruthy();
    const reason = await reasonField();
    await waitFor(() => expect(document.activeElement).toBe(reason));
    expect(reason).toHaveProperty("value", "");
    const warning = (await leadFinding()).closest(".callout");
    expect(reason.getAttribute("aria-describedby")).toContain(warning?.id);
    expect(count(sent, "GET /communication-reviews/r-1")).toBe(0);
    expect(
      screen.queryByRole("dialog", { name: "Send email with reason?" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Send with a recorded exception" }),
    ).toBeNull();
  });

  it("reads the review once from an older server that only names it", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": () => refused({}, null),
      "GET /communication-reviews/r-1": () => jsonResponse(REVIEW),
    });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    expect(await leadFinding()).toBeTruthy();
    expect(count(sent, "GET /communication-reviews/r-1")).toBe(1);
  });

  it("keeps the draft and offers a retry when that read fails", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": () => refused({}, null),
      "GET /communication-reviews/r-1": () =>
        problemResponse({ code: "internal", title: "Down" }, 500),
    });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    expect(
      await screen.findByText(/The check result was not loaded/),
    ).toBeTruthy();
    expect(screen.getByLabelText("Subject")).toHaveProperty(
      "value",
      "Your quote",
    );
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() =>
      expect(count(sent, "GET /communication-reviews/r-1")).toBe(2),
    );
  });

  it("confirms with the reason, the inline warning and the review revision", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": refused,
      "POST /communication-reviews/r-1/direct-send": () =>
        jsonResponse({ ...activity202, id: "act-9" }, 201),
    });
    render(<Composer onClose={onClose} />);
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: WHY_ASK })).toBeNull(),
    );
    await fillInline(user);
    await pressSend(user);
    await user.type(await reasonField(), "  They asked on the phone.  ");
    await user.click(await sendAnyway());

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const decisions = sent.filter(
      (entry) => entry.key === "POST /communication-reviews/r-1/direct-send",
    );
    expect(decisions).toHaveLength(1);
    expect(decisions[0]?.body).toEqual({
      reason_code: "other",
      explanation: "They asked on the phone.",
      warning_version: "inline-v1",
      assessment_revision: "rev-1",
      shown_reasons: ["marketing_objection", "no_compatible_evidence"],
      acknowledged: true,
    });
    expect(screen.getByText("Queued as a recorded exception.")).toBeTruthy();
  });

  // Each stray event alone, since the button's own press latch would hold
  // back any event that follows a first one.
  it.each([
    [
      "a click with no press behind it",
      (button: HTMLElement) => fireEvent.click(button, { detail: 1 }),
    ],
    [
      "the repeat of a key held since Send",
      (button: HTMLElement) => {
        fireEvent.keyDown(button, { key: "Enter", repeat: true });
        fireEvent.click(button, { detail: 0 });
      },
    ],
    [
      "the second click of the burst that pressed Send",
      (button: HTMLElement) => {
        fireEvent.mouseDown(button, { detail: 2 });
        fireEvent.click(button, { detail: 2 });
      },
    ],
  ])("ignores %s on Send anyway", async (_stray, stray) => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": refused,
      "POST /communication-reviews/r-1/direct-send": () =>
        jsonResponse({ ...activity202, id: "act-9" }, 201),
    });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    await user.type(await reasonField(), "Line one{Enter}line two");
    stray(await sendAnyway());
    // Only a press after the last edit sends, so the decision carries it.
    await user.type(await reasonField(), ".");
    await user.click(await sendAnyway());
    await waitFor(() =>
      expect(count(sent, "POST /communication-reviews/r-1/direct-send")).toBe(
        1,
      ),
    );
    expect(
      sent.find(
        (entry) => entry.key === "POST /communication-reviews/r-1/direct-send",
      )?.body,
    ).toMatchObject({ explanation: "Line one\nline two." });
  });

  it("sends the same key on a retried send and a new one after an edit", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": () =>
        problemResponse({ code: "internal", title: "Down" }, 500),
    });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    await waitFor(() => expect(count(sent, "POST /emails")).toBe(1));
    await pressSend(user);
    await waitFor(() => expect(count(sent, "POST /emails")).toBe(2));
    writeMessage("Body", "The prices you asked for, corrected.");
    await pressSend(user);
    await waitFor(() => expect(count(sent, "POST /emails")).toBe(3));
    const keys = sent
      .filter((entry) => entry.key === "POST /emails")
      .map((entry) => entry.headers.get("Idempotency-Key"));
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
  });

  it("sends the same key on a retried Send anyway and a new one after the reason changes", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": refused,
      "POST /communication-reviews/r-1/direct-send": () =>
        problemResponse({ code: "internal", title: "Down" }, 500),
    });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    await user.type(await reasonField(), "Asked by phone.");
    const decisions = () =>
      sent.filter(
        (entry) => entry.key === "POST /communication-reviews/r-1/direct-send",
      );
    await user.click(await sendAnyway());
    await waitFor(() => expect(decisions()).toHaveLength(1));
    await user.click(await sendAnyway());
    await waitFor(() => expect(decisions()).toHaveLength(2));
    await user.type(await reasonField(), " Twice.");
    await user.click(await sendAnyway());
    await waitFor(() => expect(decisions()).toHaveLength(3));
    const keys = decisions().map((entry) =>
      entry.headers.get("Idempotency-Key"),
    );
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
  });

  it("sends one decision for a double press while it is in flight", async () => {
    const user = userEvent.setup();
    let answer: (response: Response) => void = () => {};
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": refused,
      "POST /communication-reviews/r-1/direct-send": () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    await user.type(await reasonField(), "Asked by phone.");
    const button = await sendAnyway();
    fireEvent.mouseDown(button, { detail: 1 });
    fireEvent.click(button, { detail: 1 });
    fireEvent.mouseDown(button, { detail: 1 });
    fireEvent.click(button, { detail: 1 });
    await waitFor(() =>
      expect(count(sent, "POST /communication-reviews/r-1/direct-send")).toBe(
        1,
      ),
    );
    answer(jsonResponse({ ...activity202, id: "act-9" }, 201));
  });

  it(
    "holds the words, the close and the draft while Send anyway is out",
    async () => {
      const user = userEvent.setup();
      let answer: (response: Response) => void = () => {};
      const sent = stubRoutes({
        "GET /me": inlineMe,
        "POST /emails": refused,
        "POST /communication-reviews/r-1/direct-send": () =>
          new Promise<Response>((resolve) => {
            answer = resolve;
          }),
      });
      const { queryClient } = render(<Composer />);
      await waitFor(() =>
        expect(screen.queryByRole("combobox", { name: WHY_ASK })).toBeNull(),
      );
      await fillInline(user);
      await pressSend(user);
      await user.type(await reasonField(), "Asked by phone.");
      await waitFor(() => {
        const last = sent
          .filter((entry) => entry.key === "PUT /mail-drafts")
          .at(-1);
        expect(last?.body).toMatchObject({
          override_explanation: "Asked by phone.",
        });
      });
      await user.click(await sendAnyway());
      await waitFor(() =>
        expect(count(sent, "POST /communication-reviews/r-1/direct-send")).toBe(
          1,
        ),
      );
      const writesBefore = count(sent, "PUT /mail-drafts");

      expect(screen.getByLabelText("Subject")).toHaveProperty("disabled", true);
      expect(screen.getByRole("button", { name: "Close" })).toHaveProperty(
        "disabled",
        true,
      );
      // An edit that still lands, such as the reason, writes no draft.
      await user.type(await reasonField(), " Twice.");
      writeMessage("Body", "Edited while it was out.");
      answer(jsonResponse({ ...activity202, id: "act-9" }, 201));

      expect(
        await screen.findByText("Queued as a recorded exception."),
      ).toBeTruthy();
      expect(count(sent, "PUT /mail-drafts")).toBe(writesBefore);
      expect(
        queryClient.getQueryData(draftKey({ type: "contact", id: "c-1" })),
      ).toBeNull();
    },
    EIGHT_WAITS_TEST_MS,
  );

  it("detaches the review on an edit and sends the edited message anew", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({ "GET /me": inlineMe, "POST /emails": refused });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    await user.type(await reasonField(), "Asked by phone.");
    await user.type(screen.getByLabelText("Subject"), " v2");
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Send anyway" })).toBeNull(),
    );
    await pressSend(user);
    await waitFor(() => expect(count(sent, "POST /emails")).toBe(2));
    expect(await reasonField()).toHaveProperty("value", "Asked by phone.");
    expect(count(sent, "POST /communication-reviews/r-1/direct-send")).toBe(0);
  });

  it(
    "keeps the reason through a typo fix, sends once on it, and starts the next message empty",
    async () => {
      const user = userEvent.setup();
      const onClose = vi.fn();
      const second = { ...REVIEW, id: "r-2", assessment_revision: "rev-2" };
      let sends = 0;
      const sent = stubRoutes({
        "GET /me": inlineMe,
        "POST /emails": () => {
          sends += 1;
          return sends === 2
            ? refused({ review_id: "r-2" }, second)
            : refused();
        },
        "POST /communication-reviews/r-2/direct-send": () =>
          jsonResponse({ ...activity202, id: "act-9" }, 201),
      });
      render(<Composer onClose={onClose} />);
      await waitFor(() =>
        expect(screen.queryByRole("combobox", { name: WHY_ASK })).toBeNull(),
      );
      await fillInline(user);
      await pressSend(user);
      await user.type(await reasonField(), "They asked for prices by phone.");
      writeMessage("Body", "The prices you asked for, as promised.");
      await waitFor(() =>
        expect(
          screen.queryByRole("button", { name: "Send anyway" }),
        ).toBeNull(),
      );
      await pressSend(user);
      expect(await reasonField()).toHaveProperty(
        "value",
        "They asked for prices by phone.",
      );
      await user.click(await sendAnyway());
      await waitFor(() => expect(onClose).toHaveBeenCalled());
      expect(
        sent
          .filter((entry) => entry.key.endsWith("/direct-send"))
          .map((entry) => [entry.key, entry.body]),
      ).toEqual([
        [
          "POST /communication-reviews/r-2/direct-send",
          expect.objectContaining({
            explanation: "They asked for prices by phone.",
            assessment_revision: "rev-2",
          }),
        ],
      ]);

      cleanup();
      await openInline(sent);
      await fillInline(user);
      await pressSend(user);
      expect(await reasonField()).toHaveProperty("value", "");
    },
    EIGHT_WAITS_TEST_MS,
  );

  it("sends the edited message when the finding is gone", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    let sends = 0;
    stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": () => {
        sends += 1;
        return sends === 1 ? refused() : jsonResponse(activity202, 202);
      },
    });
    render(<Composer onClose={onClose} />);
    await waitFor(() =>
      expect(screen.queryByRole("combobox", { name: WHY_ASK })).toBeNull(),
    );
    await fillInline(user);
    await pressSend(user);
    await sendAnyway();
    writeMessage("Body", "A plain reply.");
    await pressSend(user);
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it(
    "asks for a fresh press when the review comes back refreshed",
    async () => {
      const user = userEvent.setup();
      let decisions = 0;
      const sent = stubRoutes({
        "GET /me": inlineMe,
        "POST /emails": refused,
        "POST /communication-reviews/r-1/direct-send": () => {
          decisions += 1;
          return decisions === 1
            ? refused({}, { ...REVIEW, assessment_revision: "rev-2" })
            : jsonResponse({ ...activity202, id: "act-9" }, 201);
        },
      });
      await openInline(sent);
      await fillInline(user);
      await pressSend(user);
      await user.type(await reasonField(), "Asked by phone.");
      await user.click(await sendAnyway());
      expect(await screen.findByText(/The email checks changed/)).toBeTruthy();
      expect(await reasonField()).toHaveProperty("value", "Asked by phone.");
      expect(decisions).toBe(1);
      await user.click(await sendAnyway());
      await waitFor(() => expect(decisions).toBe(2));
      const second = sent.filter(
        (entry) => entry.key === "POST /communication-reviews/r-1/direct-send",
      )[1];
      expect(second?.body).toMatchObject({ assessment_revision: "rev-2" });
    },
    EIGHT_WAITS_TEST_MS,
  );

  it(
    "shows the country a refusal was judged under, and the refreshed one",
    async () => {
      const user = userEvent.setup();
      const judged = (
        country: string | undefined,
        source: string,
        revision: string,
      ) => ({
        ...REVIEW,
        assessment_revision: revision,
        refusals: [
          {
            address: "a@x.com",
            reason_code: "marketing_objection",
            ...(country ? { country } : {}),
            country_source: source,
          },
        ],
      });
      let decisions = 0;
      const sent = stubRoutes({
        "GET /me": inlineMe,
        "POST /emails": () =>
          refused({}, judged("de", "contact_address", "rev-1")),
        "POST /communication-reviews/r-1/direct-send": () => {
          decisions += 1;
          return decisions === 1
            ? refused({}, judged(undefined, "sender_cleared", "rev-2"))
            : jsonResponse({ ...activity202, id: "act-9" }, 201);
        },
      });
      await openInline(sent);
      await fillInline(user);
      await pressSend(user);
      expect(
        await screen.findByText("Country: Germany, from the contact’s address"),
      ).toBeTruthy();
      await user.type(await reasonField(), "Asked by phone.");
      await user.click(await sendAnyway());
      expect(
        await screen.findByText(
          "Country: none, cleared by the sender for this message",
        ),
      ).toBeTruthy();
      expect(
        screen.queryByText("Country: Germany, from the contact’s address"),
      ).toBeNull();
      await user.click(await sendAnyway());
      await waitFor(() => expect(decisions).toBe(2));
      const second = sent.filter(
        (entry) => entry.key === "POST /communication-reviews/r-1/direct-send",
      )[1];
      expect(second?.body).toMatchObject({ assessment_revision: "rev-2" });
    },
    EIGHT_WAITS_TEST_MS,
  );

  it("keeps the reason in the saved draft and restores it", async () => {
    const user = userEvent.setup();
    const saved = {
      id: "md-1",
      anchor_type: "contact",
      anchor_id: "c-1",
      to: ["a@x.com"],
      cc: [],
      bcc: [],
      subject: "Your quote",
      body: "The prices you asked for.",
      html_body: "<p>The prices you asked for.</p>",
      override_explanation: "They asked on the phone.",
      version: 1,
      agent_drafted: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "GET /mail-drafts": () => jsonResponse(saved),
      "PUT /mail-drafts": () => jsonResponse({ ...saved, version: 2 }),
      "POST /emails": refused,
    });
    await openInline(sent);
    await screen.findByText("Saved draft restored");
    await pressSend(user);
    const reason = await reasonField();
    expect(reason).toHaveProperty("value", "They asked on the phone.");
    await user.type(reason, " Twice.");
    await waitFor(() => {
      const last = sent
        .filter((entry) => entry.key === "PUT /mail-drafts")
        .at(-1);
      expect(last?.body).toMatchObject({
        override_explanation: "They asked on the phone. Twice.",
      });
      expect(last?.headers.get("If-Match")).toBeTruthy();
    });
  });

  it("offers no Send anyway where the server offers no decision", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": () => refused({ available_actions: [] }),
    });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    expect(
      await screen.findByText(
        "Edit the message or remove this recipient to send it.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Send anyway" })).toBeNull();
    expect(screen.queryByLabelText(/Reason for sending/)).toBeNull();
  });

  it("offers no Send anyway for a warning version this build does not know", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /me": inlineMe,
      "POST /emails": () =>
        refused(
          {},
          {
            ...REVIEW,
            warnings: [
              REVIEW.warnings[0],
              { version: "inline-v2", text: "Newer words." },
            ],
          },
        ),
    });
    await openInline(sent);
    await fillInline(user);
    await pressSend(user);
    await leadFinding();
    expect(screen.queryByRole("button", { name: "Send anyway" })).toBeNull();
    expect(screen.queryByText(INLINE_TEXT)).toBeNull();
  });
});
