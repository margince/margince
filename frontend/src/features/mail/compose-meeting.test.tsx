// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { writeMessage } from "../../design-system/richtext-testing";
import { ToastProvider } from "../../design-system/toast";
import { meFixture } from "../../testing/mefixture";
import {
  installComposeClock,
  jsonResponse,
  pickWhy,
  pressSend,
  problemResponse,
  render,
  type Sent,
  stubRoutes,
  WHY_ASK,
} from "./compose.testkit";
import { MEETING_NOTE } from "./composemeeting";
import { ComposeWindow } from "./composewindow";

// The reason picker offers five answers. "Met in person" notes the meeting
// on the contact before the send. The line under the pick says what the answer
// does: only Marketing adds an unsubscribe link.

const MEETING = "POST /contacts/c-1/consent/qualifying-events";

// A rep who may write the contact, which noting a meeting on it needs.
const CONTACT_WRITER = {
  "GET /me": () =>
    jsonResponse(
      meFixture({
        allow: {
          activity: ["read", "create", "update"],
          contact: ["read", "update"],
        },
      }),
    ),
};

function Composer({
  entityType = "contact",
}: Readonly<{ entityType?: string }>) {
  return (
    <ToastProvider>
      <ComposeWindow
        entityType={entityType as "contact"}
        entityId="c-1"
        contactId="c-1"
        recordAddress="a@x.com"
        open
        onClose={() => {}}
      />
    </ToastProvider>
  );
}

async function openAndWrite(
  user: UserEvent,
  entityType?: string,
  to = "a@x.com",
) {
  render(<Composer entityType={entityType} />);
  await screen.findByRole("combobox", { name: WHY_ASK });
  await user.type(screen.getByLabelText("To"), to);
  await user.tab();
  await user.type(screen.getByLabelText("Subject"), "Good to meet you");
  writeMessage("Body", "As promised at the fair, here is the overview.");
}

const count = (sent: Sent[], key: string) =>
  sent.filter((entry) => entry.key === key).length;

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("we met in person", () => {
  it("asks nothing more than the pick", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes(CONTACT_WRITER);
    await openAndWrite(user);
    await pickWhy(user, "Met in person");

    expect(screen.queryByRole("textbox", { name: /where/i })).toBeNull();
    expect(screen.queryByLabelText("Date")).toBeNull();
  });

  it("notes the meeting first, then sends it as a requested follow-up", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      ...CONTACT_WRITER,
      [MEETING]: () => jsonResponse({ id: "q-1" }, 201),
    });
    await openAndWrite(user);
    await pickWhy(user, "Met in person");
    await pressSend(user);

    await waitFor(() => expect(count(sent, "POST /emails")).toBe(1));
    const keys = sent.map((entry) => entry.key);
    expect(keys.indexOf(MEETING)).toBeLessThan(keys.indexOf("POST /emails"));
    expect(sent.find((entry) => entry.key === MEETING)?.body).toMatchObject({
      kind: "in_person",
      note: MEETING_NOTE,
    });
    expect(
      sent.find((entry) => entry.key === "POST /emails")?.body,
    ).toMatchObject({ communication_context: "requested_followup" });
  });

  it("notes one meeting once when the send is pressed again after a refusal", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      ...CONTACT_WRITER,
      [MEETING]: () => jsonResponse({ id: "q-1" }, 201),
      "POST /emails": () =>
        problemResponse({ title: "Unavailable", status: 503 }, 503),
    });
    await openAndWrite(user);
    await pickWhy(user, "Met in person");
    await pressSend(user);
    await waitFor(() => expect(count(sent, "POST /emails")).toBe(1));
    await pressSend(user);

    await waitFor(() => expect(count(sent, "POST /emails")).toBe(2));
    expect(count(sent, MEETING)).toBe(1);
  });

  it("sends nothing when the meeting could not be noted", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      ...CONTACT_WRITER,
      [MEETING]: () =>
        problemResponse({ title: "Forbidden", status: 403 }, 403),
    });
    await openAndWrite(user);
    await pickWhy(user, "Met in person");
    await pressSend(user);

    expect(
      await screen.findByText(
        "The meeting was not saved, so nothing was sent. Try again.",
      ),
    ).toBeTruthy();
    expect(count(sent, "POST /emails")).toBe(0);
  });

  it("is not offered where the composer has no contact to note it on", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes(CONTACT_WRITER);
    await openAndWrite(user, "company");
    await user.click(screen.getByRole("combobox", { name: WHY_ASK }));

    expect(screen.queryByRole("option", { name: "Met in person" })).toBe(null);
    expect(
      screen.getByRole("option", { name: "They asked for it" }),
    ).toBeTruthy();
  });
});

describe("a meeting is noted only on the contact the mail goes to", () => {
  it("is not offered when the mail goes to someone other than the record", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes(CONTACT_WRITER);
    await openAndWrite(user, "contact", "bob@x.com");
    await user.click(screen.getByRole("combobox", { name: WHY_ASK }));

    expect(screen.queryByRole("option", { name: "Met in person" })).toBe(null);
  });

  it("is not offered once someone else is copied", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes(CONTACT_WRITER);
    await openAndWrite(user);
    await user.click(screen.getByRole("button", { name: "Cc and Bcc" }));
    await user.type(screen.getByLabelText("Cc"), "bob@x.com");
    await user.tab();
    await user.click(screen.getByRole("combobox", { name: WHY_ASK }));

    expect(screen.queryByRole("option", { name: "Met in person" })).toBe(null);
  });
});

describe("what each answer does to the message", () => {
  it("says only Marketing adds an unsubscribe link", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes();
    await openAndWrite(user);

    await pickWhy(user, "Marketing");
    expect(
      await screen.findByText(
        /Adds an unsubscribe link and the company footer/,
      ),
    ).toBeTruthy();

    await pickWhy(user, "Existing customer");
    expect(
      await screen.findByText("Service mail, no unsubscribe link."),
    ).toBeTruthy();
    expect(screen.queryByText(/Adds an unsubscribe link/)).toBe(null);
  });
});
