/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TimelineActions } from "../records/timelineactions";
import { ChannelReplyAction } from "./compose";
import {
  type Activity,
  activity202,
  jsonResponse,
  render,
  stubRoutes,
} from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const ANNA = {
  id: "p-1",
  full_name: "Anna Weber",
  primary_email: "anna@example.test",
  reachability: [],
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function row(over: Partial<Activity> & Pick<Activity, "id" | "kind">) {
  return { ...activity202, occurred_at: "2026-07-01T09:30:00Z", ...over };
}

function onContactPage(activity: Activity, contact: object = ANNA) {
  stubRoutes({
    "GET /contacts/p-1": () => jsonResponse(contact),
    [`GET /activities/${activity.id}`]: () => jsonResponse(activity),
  });
  render(
    <TimelineActions
      activity={activity}
      entityType="contact"
      entityId="p-1"
      contactId="p-1"
    />,
  );
}

describe("what a timeline row offers to write", () => {
  it.each([
    row({ id: "a-call", kind: "call", subject: "Voicemail" }),
    row({ id: "a-note", kind: "note", subject: "Budget moved to Q4" }),
    row({ id: "a-meet", kind: "meeting", subject: "Site visit" }),
  ])(
    "opens a new email about a $kind row, never a reply to it",
    async (activity) => {
      const user = userEvent.setup();
      onContactPage(activity);

      await user.click(
        await screen.findByRole("button", { name: "Write email" }),
      );
      expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();

      const dialog = await screen.findByRole("dialog", { name: /^New email/ });
      expect(
        await within(dialog).findByText(
          new RegExp(`^About: ${activity.subject} · `),
        ),
      ).toBeTruthy();
      expect(within(dialog).queryByText(/Replying to/)).toBeNull();
      expect(within(dialog).queryByText(/Following up on/)).toBeNull();
      expect(
        within(dialog).getByRole("button", { name: "Draft email" }),
      ).toBeTruthy();
      expect(
        within(dialog).getByRole("textbox", { name: "Purpose of the email" }),
      ).toBeTruthy();
      expect(
        within(dialog).queryByRole("textbox", { name: "Purpose of the reply" }),
      ).toBeNull();
    },
  );

  it("keeps an email row a reply, and the dialog says what it answers", async () => {
    const user = userEvent.setup();
    const email = row({
      id: "a-mail",
      kind: "email",
      subject: "Angebot",
      direction: "inbound",
    });
    onContactPage(email);

    await user.click(await screen.findByRole("button", { name: "Reply" }));
    expect(screen.queryByRole("button", { name: "Write email" })).toBeNull();

    const dialog = await screen.findByRole("dialog", { name: /^Re: Angebot/ });
    expect(
      await within(dialog).findByText(/^Replying to “Angebot” · /),
    ).toBeTruthy();
    expect(within(dialog).queryByText(/^About: /)).toBeNull();
    expect(
      within(dialog).getByRole("button", { name: "Draft reply" }),
    ).toBeTruthy();
  });

  it("offers nothing to write on a note when the contact has no address", async () => {
    onContactPage(row({ id: "a-note", kind: "note", subject: "Called back" }), {
      ...ANNA,
      primary_email: null,
    });

    // Relink proves the row drew; the missing verb is the address gate.
    expect(await screen.findByRole("button", { name: "Relink" })).toBeTruthy();
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Write email" })).toBeNull(),
    );
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
  });

  it("offers Write email on the same note once the contact has an address", async () => {
    onContactPage(row({ id: "a-note", kind: "note", subject: "Called back" }));

    expect(
      await screen.findByRole("button", { name: "Write email" }),
    ).toBeTruthy();
  });

  it.each([
    row({
      id: "t-system",
      kind: "task",
      subject: "1 forecast input needs attention",
      captured_by: "system",
      automatic_kind: "product",
    }),
    row({
      id: "t-loose",
      kind: "task",
      subject: "Check the contract",
      links: [],
    }),
  ])("offers no email on a task row: $subject", async (task) => {
    onContactPage(task);

    expect(await screen.findByRole("button", { name: "Relink" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Write email" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
  });

  it("draws no verb for a task handed to the reply action directly", async () => {
    stubRoutes({ "GET /contacts/p-1": () => jsonResponse(ANNA) });
    const { container } = render(
      <ChannelReplyAction
        activityId="t-1"
        kind="task"
        entityType="contact"
        entityId="p-1"
        contactId="p-1"
      />,
    );

    expect(container.childElementCount).toBe(0);
  });
});
