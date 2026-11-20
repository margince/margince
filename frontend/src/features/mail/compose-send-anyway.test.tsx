// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ASYNC_UTIL_TIMEOUT_MS,
  SLOWEST_MEASURED_TEST_MS,
} from "../../../vitest.budget";
import { writeMessage } from "../../design-system/richtext-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { isPreviewDoor } from "../../testing/sendpermission.testkit";
import {
  activity202,
  installComposeClock,
  jsonResponse,
  pickWhy,
  pressSend,
  problemResponse,
  render,
  type Sent,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
} from "./compose.testkit";
import { draftKey } from "./composesaveddraftcalls";
import { ComposeWindow } from "./composewindow";

// The email composer decides at Send. Nothing is asked while the rep writes,
// and Send is the check. A refusal comes back as a review, drawn in place with
// one reason and Send anyway.

// The in-flight and refreshed-review tests each chain eight waits, past the
// default ceiling.
const EIGHT_WAITS_TEST_MS =
  ASYNC_UTIL_TIMEOUT_MS * 8 + SLOWEST_MEASURED_TEST_MS;

const INLINE_TEXT = "Why send this one message anyway?";

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
    { version: "inline-v2", text: INLINE_TEXT },
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

// A new message to the contact: nothing it answers, so it asks why.
async function openComposer() {
  render(<Composer />);
  await screen.findByRole("combobox", { name: WHY_ASK });
}

async function fillMessage(user: UserEvent) {
  await user.type(screen.getByLabelText("To"), "a@x.com");
  await user.tab();
  await user.type(screen.getByLabelText("Subject"), "Your quote");
  writeMessage("Body", "The prices you asked for.");
  await pickWhy(user, WHY_LABEL.requestedFollowup);
}

// The callout's title. The details behind the count list only the findings
// after it.
const leadFinding = () => screen.findByText("a@x.com objected to marketing.");
const reasonField = () =>
  screen.findByLabelText(/Why send this one message anyway\?/);
const sendAnyway = () => screen.findByRole("button", { name: "Send anyway" });

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the email composer that decides at Send", () => {
  it("asks nothing while the rep opens, types and the draft saves", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes();
    await openComposer();
    await fillMessage(user);
    await waitFor(() =>
      expect(count(sent, "PUT /mail-drafts")).toBeGreaterThan(0),
    );
    expect(previews(sent)).toBe(0);
    expect(count(sent, "POST /lawful-basis-confirmations")).toBe(0);
  });

  it("sends a clean message with one request and closes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    render(<Composer onClose={onClose} />);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillMessage(user);
    await pressSend(user);
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(count(sent, "POST /emails")).toBe(1);
    const body = sent.find((entry) => entry.key === "POST /emails")?.body;
    expect(body).toHaveProperty("communication_context", "requested_followup");
    expect(previews(sent)).toBe(0);
  });

  // Consent is owed to everyone who receives the message, so the server
  // decides on the blind copy as well.
  it("puts a blind copy in the request the server decides on", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({ "POST /emails": refused });
    await openComposer();
    await fillMessage(user);
    await user.click(screen.getByRole("button", { name: "Cc and Bcc" }));
    await user.type(screen.getByLabelText("Bcc"), "quiet@x.com");
    await user.tab();
    await pressSend(user);
    await leadFinding();
    expect(
      sent.find((entry) => entry.key === "POST /emails")?.body,
    ).toMatchObject({ to: ["a@x.com"], bcc: ["quiet@x.com"] });
  });

  it("closes a scheduled send with the scheduled note", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "POST /emails": () => jsonResponse(activity202, 201),
    });
    render(<Composer />);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillMessage(user);
    await pressSend(user);
    expect(
      await screen.findByText("Email scheduled. It has not been sent yet."),
    ).toBeTruthy();
  });

  it("draws a refusal from the embedded review without asking again", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({ "POST /emails": refused });
    await openComposer();
    await fillMessage(user);
    await pressSend(user);

    expect(await leadFinding()).toBeTruthy();
    expect(await reasonField()).toBeTruthy();
    // The finding is the whole warning: no explanation paragraph beneath it.
    expect(screen.queryByText(/grants no permission/)).toBeNull();
    expect(screen.getByText("1 more finding")).toBeTruthy();
    expect(
      screen.getByText(/^No reason to contact b@x\.com is on record\./),
    ).toBeTruthy();
    expect(screen.getAllByText(/a@x\.com objected to marketing/)).toHaveLength(
      1,
    );
    expect(screen.getByLabelText("Subject")).toHaveProperty(
      "value",
      "Your quote",
    );
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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "POST /emails": () => refused({}, null),
      "GET /communication-reviews/r-1": () => jsonResponse(REVIEW),
    });
    await openComposer();
    await fillMessage(user);
    await pressSend(user);
    expect(await leadFinding()).toBeTruthy();
    expect(count(sent, "GET /communication-reviews/r-1")).toBe(1);
  });

  it("keeps the draft and offers a retry when that read fails", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "POST /emails": () => refused({}, null),
      "GET /communication-reviews/r-1": () =>
        problemResponse({ code: "internal", title: "Down" }, 500),
    });
    await openComposer();
    await fillMessage(user);
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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /emails": refused,
      "POST /communication-reviews/r-1/direct-send": () =>
        jsonResponse({ ...activity202, id: "act-9" }, 201),
    });
    render(<Composer onClose={onClose} />);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillMessage(user);
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
      warning_version: "inline-v2",
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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "POST /emails": refused,
      "POST /communication-reviews/r-1/direct-send": () =>
        jsonResponse({ ...activity202, id: "act-9" }, 201),
    });
    await openComposer();
    await fillMessage(user);
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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "POST /emails": () =>
        problemResponse({ code: "internal", title: "Down" }, 500),
    });
    await openComposer();
    await fillMessage(user);
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

  async function sendAnywayTwice(changeReason: boolean) {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "POST /emails": refused,
      "POST /communication-reviews/r-1/direct-send": () =>
        problemResponse({ code: "internal", title: "Down" }, 500),
    });
    await openComposer();
    await fillMessage(user);
    await pressSend(user);
    const reason = await reasonField();
    await user.type(reason, "Asked by phone.");
    const decisions = () =>
      sent.filter(
        (entry) => entry.key === "POST /communication-reviews/r-1/direct-send",
      );
    await user.click(await sendAnyway());
    await waitFor(() => expect(decisions()).toHaveLength(1));
    if (changeReason) {
      await user.type(reason, " Twice.");
    }
    await user.click(await sendAnyway());
    await waitFor(() => expect(decisions()).toHaveLength(2));
    return decisions().map((entry) => entry.headers.get("Idempotency-Key"));
  }

  it("sends the same key on a retried Send anyway", async () => {
    const keys = await sendAnywayTwice(false);
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
  });

  it("sends a new key on a Send anyway after the reason changes", async () => {
    const keys = await sendAnywayTwice(true);
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).not.toBe(keys[0]);
  });

  it("sends one decision for a double press while it is in flight", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    let answer: (response: Response) => void = () => {};
    const sent = stubRoutes({
      "POST /emails": refused,
      "POST /communication-reviews/r-1/direct-send": () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    });
    await openComposer();
    await fillMessage(user);
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
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      let answer: (response: Response) => void = () => {};
      const sent = stubRoutes({
        "POST /emails": refused,
        "POST /communication-reviews/r-1/direct-send": () =>
          new Promise<Response>((resolve) => {
            answer = resolve;
          }),
      });
      const { queryClient } = render(<Composer />);
      await screen.findByRole("combobox", { name: WHY_ASK });
      await fillMessage(user);
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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({ "POST /emails": refused });
    await openComposer();
    await fillMessage(user);
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
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const onClose = vi.fn();
      const second = { ...REVIEW, id: "r-2", assessment_revision: "rev-2" };
      let sends = 0;
      const sent = stubRoutes({
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
      await screen.findByRole("combobox", { name: WHY_ASK });
      await fillMessage(user);
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
      await openComposer();
      await fillMessage(user);
      await pressSend(user);
      expect(await reasonField()).toHaveProperty("value", "");
    },
    EIGHT_WAITS_TEST_MS,
  );

  it("sends the edited message when the finding is gone", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    let sends = 0;
    stubRoutes({
      "POST /emails": () => {
        sends += 1;
        return sends === 1 ? refused() : jsonResponse(activity202, 202);
      },
    });
    render(<Composer onClose={onClose} />);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillMessage(user);
    await pressSend(user);
    await sendAnyway();
    writeMessage("Body", "A plain reply.");
    await pressSend(user);
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it(
    "asks for a fresh press when the review comes back refreshed",
    async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      let decisions = 0;
      const sent = stubRoutes({
        "POST /emails": refused,
        "POST /communication-reviews/r-1/direct-send": () => {
          decisions += 1;
          return decisions === 1
            ? refused({}, { ...REVIEW, assessment_revision: "rev-2" })
            : jsonResponse({ ...activity202, id: "act-9" }, 201);
        },
      });
      await openComposer();
      await fillMessage(user);
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
    "keeps the warning to its finding, and asks again on a refreshed review",
    async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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
        "POST /emails": () =>
          refused({}, judged("de", "contact_address", "rev-1")),
        "POST /communication-reviews/r-1/direct-send": () => {
          decisions += 1;
          return decisions === 1
            ? refused({}, judged(undefined, "sender_cleared", "rev-2"))
            : jsonResponse({ ...activity202, id: "act-9" }, 201);
        },
      });
      await openComposer();
      await fillMessage(user);
      await pressSend(user);
      expect(await leadFinding()).toBeTruthy();
      // The warning is its first line: the country it was judged under is not
      // repeated beneath it.
      expect(screen.queryByText(/^Country:/)).toBeNull();
      await user.type(await reasonField(), "Asked by phone.");
      await user.click(await sendAnyway());
      expect(await screen.findByText(/The email checks changed/)).toBeTruthy();
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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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
      communication_context: "requested_followup",
      override_explanation: "They asked on the phone.",
      version: 1,
      agent_drafted: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(saved),
      "PUT /mail-drafts": () => jsonResponse({ ...saved, version: 2 }),
      "POST /emails": refused,
    });
    await openComposer();
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
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "POST /emails": () => refused({ available_actions: [] }),
    });
    await openComposer();
    await fillMessage(user);
    await pressSend(user);
    expect(
      await screen.findByText(
        "Edit the message or remove this recipient to send it.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Send anyway" })).toBeNull();
    expect(
      screen.queryByLabelText(/Why send this one message anyway\?/),
    ).toBeNull();
  });

  it("offers no Send anyway for a warning version this build does not know", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "POST /emails": () =>
        refused(
          {},
          {
            ...REVIEW,
            warnings: [
              REVIEW.warnings[0],
              { version: "inline-v3", text: "Newer words." },
            ],
          },
        ),
    });
    await openComposer();
    await fillMessage(user);
    await pressSend(user);
    await leadFinding();
    expect(screen.queryByRole("button", { name: "Send anyway" })).toBeNull();
    expect(screen.queryByLabelText(INLINE_TEXT)).toBeNull();
  });
});
