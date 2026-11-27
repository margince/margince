/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { allowedPreview } from "../../testing/sendpermission.testkit";
import {
  fillSendableForm,
  installComposeClock,
  jsonResponse,
  pressSend,
  problemResponse,
  render,
  stubRoutes,
  WHY_ASK,
} from "./compose.testkit";
import { refusalIsCurrent } from "./composesendrequest";
import { ComposeWindow } from "./composewindow";

// After a refused Send the composer shows that refusal, and only while it is
// about the message on screen.

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function refusedSend(review: unknown) {
  return () =>
    problemResponse(
      {
        code: "consent_not_granted",
        title: "Conflict",
        details: {
          review_id: "r-1",
          available_actions: [],
          refusals: [],
          review,
        },
      },
      409,
    );
}

function openComposer() {
  return render(
    <ComposeWindow
      activityId="act-1"
      entityType="contact"
      entityId="p-1"
      open
      onClose={vi.fn()}
    />,
  );
}

describe("a refused send", () => {
  // A finding whose reason this build cannot read names no cause it does not
  // know, and the draft stays for the rep to change.
  it("says the message cannot be sent, without claiming consent, when the reason is unreadable", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "POST /activities/act-1/send-email": refusedSend({
        id: "r-1",
        state: "open",
        reason_code: "something_new",
        refusals: [{ address: "a@x.com", reason_code: "something_new" }],
      }),
    });
    openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await pressSend(user);

    const finding = await screen.findByText(
      "This message cannot be sent to a@x.com as it stands.",
    );
    expect(finding.closest(".callout")).not.toHaveTextContent(/consent/i);
    expect(screen.getByLabelText("Subject")).toHaveValue("Hi there");
  });
});

describe("whether a refusal is about the message on screen", () => {
  type Request = Parameters<typeof refusalIsCurrent>[1];
  const answerAtSend = allowedPreview(["a@x.com"]);
  const sent: Request = {
    activityId: "act-1",
    isChannelReply: false,
    mail: {
      subject: "Hi",
      body: "Words",
      to: ["a@x.com"],
      attachment_ids: ["f-1"],
      mail_draft_id: undefined,
    },
    channelBody: { body: "Words", attachment_ids: ["f-1"] },
    links: [],
    answerAtSend,
  };

  it("is, while nothing the request carries has changed", () => {
    // A draft id arriving after the press is bookkeeping, not a new message.
    const now = { ...sent, mail: { ...sent.mail, mail_draft_id: "d-1" } };
    expect(refusalIsCurrent(sent, now, answerAtSend)).toBe(true);
  });

  it("is not, once a file is added", () => {
    const now = {
      ...sent,
      mail: { ...sent.mail, attachment_ids: ["f-1", "f-2"] },
      channelBody: { ...sent.channelBody, attachment_ids: ["f-1", "f-2"] },
    };
    expect(refusalIsCurrent(sent, now, answerAtSend)).toBe(false);
  });

  it("retains the reason form through a background preview refresh", () => {
    expect(
      refusalIsCurrent(sent, sent, allowedPreview(["a@x.com"]), true),
    ).toBe(true);
  });

  it("is not, once the check answers anew", () => {
    expect(refusalIsCurrent(sent, sent, allowedPreview(["a@x.com"]))).toBe(
      false,
    );
  });
});

it("carries the selected deal in the queued message", async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const sent = stubRoutes({ "POST /emails": () => jsonResponse({}, 201) });
  render(
    <ComposeWindow
      entityType="contact"
      entityId="p-1"
      initialDealId="deal-one"
      open
      onClose={vi.fn()}
    />,
  );
  await screen.findByRole("combobox", { name: WHY_ASK });
  await fillSendableForm(user);
  await pressSend(user);
  await waitFor(() =>
    expect(
      sent.find((call) => call.key === "POST /emails")?.body,
    ).toMatchObject({ evidence: { deal_id: "deal-one" } }),
  );
});
