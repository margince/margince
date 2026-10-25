/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  messageBox,
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { TimelineActions } from "../records/timelineactions";
import { ChannelReplyAction, ComposeModal } from "./compose";
import {
  type Activity,
  activity202,
  jsonResponse,
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

// The Telegram reply reuses ComposeModal's confirm-first send, its consent
// rendering, and its post-send refresh wholesale. Only the wire target and the
// fields a channel has no concept of differ from the mail path.
describe("ComposeModal — channel reply", () => {
  const telegramActivity: Activity = {
    ...activity202,
    id: "act-1",
    kind: "message",
    channel_provider: "telegram",
    subject: null,
    direction: "inbound",
  };

  it("posts to send-message for a channel activity", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /activities/act-1/send-message": () =>
        jsonResponse(
          { ...telegramActivity, direction: "outbound", body: "On my way." },
          202,
        ),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        kind="message"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    writeMessage("Body", "On my way.");
    await pickWhy(WHY_LABEL.requestedFollowup);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const req = sent.find(
      (r) => r.key === "POST /activities/act-1/send-message",
    );
    // No `to`, `subject`, `cc`, or `draft_ref`. SendMessageRequest carries
    // only these two fields, the wire shape a channel send accepts.
    expect(req?.body).toEqual({
      body: "On my way.",
      communication_context: "requested_followup",
    });
    // ADR-0055: the human's own click is the approval on both send paths.
    expect(req?.headers.get("X-Approval-Token")).toBeNull();
    expect(req?.headers.get("Idempotency-Key")).toBeNull();
  });

  it("hides subject and cc for a channel reply", async () => {
    stubRoutes();
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    writeMessage("Body", "On my way.");

    // The interaction proves the surface is otherwise usable; the missing
    // fields are the point of the test, not an accident of an unrendered form.
    expect(messageText("Body")).toBe("On my way.");
    expect(screen.queryByLabelText("Subject")).toBeNull();
    expect(screen.queryByLabelText("Cc")).toBeNull();
    // A channel has no addressable "to" either. The server resolves the
    // recipient from the conversation's own channel identity.
    expect(screen.queryByLabelText("To")).toBeNull();
  });

  it("names the channel it is about to send on, and says the send is final", async () => {
    stubRoutes();
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });

    // Confirming an irreversible send under the name of a channel this
    // message will never travel on is a lie the rep cannot check.
    expect(screen.getByText("Send message")).toBeTruthy();
    expect(screen.queryByText("Send email")).toBeNull();
    // The heading is the only place the channel is named, so it cannot also
    // be the only place that states the send is irreversible. The modal chrome
    // around it is a tier dot and two buttons.
    expect(screen.getByText(/cannot be undone/)).toBeTruthy();
  });

  it("gives the message box an accessible name of its own", async () => {
    stubRoutes();
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });

    // getByLabelText resolves labels only, and a placeholder does not satisfy
    // it. So this holds the box to a name that survives the rep typing into
    // it, the moment the placeholder disappears.
    expect(messageBox("Body")).toBeTruthy();
  });

  it("keeps the drafted text when consent is refused", async () => {
    stubRoutes({
      "POST /activities/act-1/send-message": () =>
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
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    writeMessage("Body", "Call me back please.");
    await pickWhy(WHY_LABEL.requestedFollowup);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/has not granted consent/i)).toBeTruthy();
    // The rep's words survive the refusal, and this test proves that, not only
    // that an error rendered. Losing a written reply to a 409 once makes a rep
    // stop trusting the surface.
    expect(messageText("Body")).toBe("Call me back please.");
  });
});

// Relink is disabled until /me answers. Once it is enabled the grants have
// settled, so an absent control is a decision and not a pending read.
async function relinkSettled() {
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Relink" }).hasAttribute("disabled"),
    ).toBe(false),
  );
}

describe("TimelineActions", () => {
  const email: Activity = {
    ...activity202,
    id: "a1",
    kind: "email",
  };
  const note: Activity = {
    ...activity202,
    id: "a2",
    kind: "note",
    links: [{ entity_type: "contact", entity_id: "c1" }],
    offers_email: true,
  };

  it("withholds Relink and the audience control on an activity the server says is not the reader's to change", async () => {
    stubRoutes();
    render(
      <TimelineActions
        activity={{ ...note, writable: false }}
        entityType="deal"
        entityId="d1"
      />,
    );
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Relink" }).hasAttribute("disabled"),
      ).toBe(true),
    );
    expect(
      screen.queryByRole("button", { name: "Change visibility" }),
    ).toBeNull();
  });

  it("offers Reply on an email row and Relink alongside it", () => {
    stubRoutes();
    render(
      <TimelineActions activity={email} entityType="deal" entityId="d1" />,
    );
    expect(screen.getByRole("button", { name: "Reply" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
  });

  it("offers Write email on a non-email row too, as the send path allows", async () => {
    // A send anchored to a note carries no RFC822 identity and starts a
    // conversation, which the backend handles. Gating this on kind === "email"
    // is what made "log a note → compose → send" unreachable in a workspace
    // whose timeline holds nothing captured from mail.
    stubRoutes();
    render(<TimelineActions activity={note} entityType="deal" entityId="d1" />);
    expect(
      await screen.findByRole("button", { name: "Write email" }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
    // Relink is always available: the row is already linked to this timeline's entity.
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
  });

  it("opens the composer anchored to a note row", async () => {
    stubRoutes();
    render(<TimelineActions activity={note} entityType="deal" entityId="d1" />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Write email" }),
    );
    expect(await screen.findByRole("dialog")).toBeTruthy();
  });

  it("offers Write email beside Open task on a task with one contact", async () => {
    stubRoutes();
    const task: Activity = { ...note, id: "t1", kind: "task" };
    render(<TimelineActions activity={task} entityType="deal" entityId="d1" />);
    expect(
      await screen.findByRole("button", { name: "Write email" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open task" })).toBeTruthy();
  });

  it("offers Open task and no email on a task the product filed", async () => {
    stubRoutes();
    const task: Activity = {
      ...note,
      id: "t2",
      kind: "task",
      automatic_kind: "product",
      offers_email: false,
    };
    render(
      <>
        <TimelineActions activity={note} entityType="deal" entityId="d1" />
        <TimelineActions activity={task} entityType="deal" entityId="d1" />
      </>,
    );
    // The note row is the positive control in the same render.
    await screen.findByRole("button", { name: "Write email" });
    expect(screen.getAllByRole("button", { name: "Write email" })).toHaveLength(
      1,
    );
    expect(screen.getByRole("button", { name: "Open task" })).toBeTruthy();
  });

  it("opens the composer when Reply is clicked", async () => {
    stubRoutes();
    render(
      <TimelineActions activity={email} entityType="deal" entityId="d1" />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Reply" }));
    // The composer mounts only once Reply opens it, headed as a reply.
    expect(await screen.findByRole("dialog", { name: /^Re: / })).toBeTruthy();
  });

  it("offers no reply when the contact is unreachable", async () => {
    // With a blocked or never-established Telegram identity, a reply box here
    // would fail only after the rep has written the message. That is worse
    // than never offering it.
    const telegram: Activity = {
      ...activity202,
      id: "a3",
      kind: "message",
      channel_provider: "telegram",
    };
    stubRoutes({
      "GET /contacts/p-1": () =>
        jsonResponse({
          id: "p-1",
          full_name: "Jane Doe",
          reachability: [
            {
              provider: "telegram",
              reachable: false,
              since: "2026-07-01T00:00:00Z",
            },
          ],
          version: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        }),
    });
    render(
      <TimelineActions
        activity={telegram}
        entityType="contact"
        entityId="p-1"
        contactId="p-1"
      />,
    );

    // Relink renders at once, since it never waits on reachability. That
    // proves the component mounted, and that the missing Reply button is the
    // gate, not an unrendered tree.
    expect(await screen.findByRole("button", { name: "Relink" })).toBeTruthy();
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Reply" })).toBeNull(),
    );
  });

  // `Write email` has to be able to SEND.
  //
  // Withholding the anchor makes the dialog match the button. It is not a
  // reply, so it must not claim to read the message it answers. But the modal
  // reads `kind: "message"` as a channel reply and posts to send-message. That
  // needs the conversation that was withheld, so Send would throw "a channel
  // reply needs the conversation it answers".
  //
  // Asserted through To and Subject, which a channel reply does not render and
  // an account-started email does. The modal's validation requires those two
  // fields when it is not a channel reply. So a mode that showed them and still
  // posted to send-message could not pass this.
  it("opens a sendable email composer when the channel message is withheld", async () => {
    // Reachable, or the action renders nothing and the case would pass over an
    // empty tree. An absent button is a different answer.
    stubRoutes({
      "GET /contacts/p-1": () =>
        jsonResponse({
          id: "p-1",
          full_name: "Jane Doe",
          reachability: [
            {
              provider: "telegram",
              reachable: true,
              since: "2026-07-01T00:00:00Z",
            },
          ],
          version: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        }),
    });
    render(
      <ChannelReplyAction
        activityId="a9"
        kind="message"
        channelProvider="telegram"
        entityType="contact"
        entityId="p-1"
        contactId="p-1"
        contentWithheld
      />,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Write email" }),
    );

    expect(await screen.findByLabelText("To")).toBeTruthy();
    expect(screen.getByLabelText("Subject")).toBeTruthy();
  });

  // An email's audience is changed from the message, in the drawer, where the
  // server states which of the two writes it would accept as `change_mode`.
  // The row must not decide that by testing `captured_by` for a `connector:`
  // prefix, which would spell a backend ownership rule in display code.
  //
  // Both shapes are asserted, because that inference would tell them apart.
  // A captured row and a hand-typed one get the same answer: no audience
  // control on the row at all.
  it("offers no audience control on an email row, whoever captured it", async () => {
    stubRoutes();
    const captured: Activity = {
      ...activity202,
      id: "a4",
      captured_by: "connector:gmail:u1",
      thread_key: "thread:abc",
    };
    const { unmount } = render(
      <TimelineActions activity={captured} entityType="deal" entityId="d1" />,
    );
    expect(screen.queryByRole("button", { name: "Share thread" })).toBeNull();
    await relinkSettled();
    expect(
      screen.queryByRole("button", { name: "Change visibility" }),
    ).toBeNull();
    // Relink still draws, so the two absences above are the gate rather than a
    // component that never mounted at all.
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
    unmount();

    const handTyped: Activity = {
      ...activity202,
      id: "a5",
      captured_by: "human:u1",
      audience: "participants",
    };
    render(
      <TimelineActions activity={handTyped} entityType="deal" entityId="d1" />,
    );
    expect(
      screen.queryByRole("button", { name: "Change visibility" }),
    ).toBeNull();
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
  });

  // A connector-captured conversation derives its audience from what each
  // importing mailbox asked for, as an email does. The per-message control
  // would open, take an answer, and refuse it with `audience_is_derived`. That
  // names a thread-level control the page does not have. A control that cannot
  // work is worse than none: someone keeping a conversation private presses it.
  it("offers no audience control on a captured message whose audience is derived", async () => {
    stubRoutes();
    render(
      <TimelineActions
        activity={{
          ...activity202,
          id: "a8",
          kind: "message",
          captured_by: "connector:ext:openchannel:1",
          thread_key: "openchannel:abc",
          audience_reason: "posture",
        }}
        entityType="deal"
        entityId="d1"
      />,
    );
    await relinkSettled();
    expect(
      screen.queryByRole("button", { name: "Change visibility" }),
    ).toBeNull();
    // The row is still actionable: this withholds one control, not the row.
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
  });

  // A successfully-linked connector message is born `workspace` with no
  // `audience_reason`. decideBirthTx stamps one only when something holds the
  // row, and limitLinkLessAudience only for a link-less one. Yet the server's
  // `activityWasImported` test still refuses a direct write on it, because an
  // import row exists whatever it derived. Gating only on `audience_reason`
  // misses this row: Visibility opens, takes an answer, and the write is
  // refused with no working control on the page (margince#4152).
  it("offers no audience control on a captured message with no audience_reason at all", () => {
    stubRoutes();
    render(
      <TimelineActions
        activity={{
          ...activity202,
          id: "a10",
          kind: "message",
          captured_by: "connector:ext:openchannel:1",
          thread_key: "openchannel:abc",
          audience_reason: null,
        }}
        entityType="deal"
        entityId="d1"
      />,
    );
    expect(screen.queryByRole("button", { name: "Visibility" })).toBeNull();
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
  });

  // `manual` is a HUMAN's own answer, not the system's, so it stays theirs to
  // change. Without this the fix above would read as "any reason at all hides
  // the control", which withholds it from every row somebody has already set.
  it("keeps the audience control on a row a human set the audience of", async () => {
    stubRoutes();
    render(
      <TimelineActions
        activity={{
          ...activity202,
          id: "a9",
          kind: "message",
          audience_reason: "manual",
        }}
        entityType="deal"
        entityId="d1"
      />,
    );
    expect(
      await screen.findByRole("button", { name: "Change visibility" }),
    ).toBeTruthy();
  });

  // A note, a call or a meeting derives no audience, so the row keeps the
  // control for every kind but email.
  it("keeps the audience control on a row that is not an email", async () => {
    stubRoutes();
    render(
      <TimelineActions
        activity={{ ...activity202, id: "a7", kind: "note" }}
        entityType="deal"
        entityId="d1"
      />,
    );
    expect(
      await screen.findByRole("button", { name: "Change visibility" }),
    ).toBeTruthy();
  });

  // `selected` is the API's third audience. Choosing it without a way to name
  // anybody is a choice the reader cannot complete. These claims pin that the
  // dialog offers it with that way.
  it("names the contacts a message is limited to and submits them as one set", async () => {
    const roster = {
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u-2",
              display_name: "Lena Fischer",
              email: "lena@demo.test",
            },
            // An agent seat, which the picker must not offer: a message is
            // limited to contacts and teams, never to an agent.
            {
              id: "u-9",
              display_name: "Margince",
              email: "agent@demo.test",
              is_agent: true,
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /teams": () =>
        jsonResponse({
          data: [{ id: "t-1", name: "Vertrieb" }],
          page: { next_cursor: null, has_more: false },
        }),
    };
    const sent = stubRoutes(roster);
    render(
      <TimelineActions
        activity={{
          ...activity202,
          id: "a6",
          // A NOTE, because this dialog is the row's and the row no longer
          // draws it for an email. The claims below are about the picker and
          // the write it sends, which are the same for every kind that still
          // has one.
          kind: "note",
          captured_by: "human:u1",
          version: 3,
        }}
        entityType="deal"
        entityId="d1"
      />,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Change visibility" }),
    );
    await userEvent.click(screen.getByLabelText(/Named people/));

    // An agent seat is not on offer.
    expect(await screen.findByLabelText(/Lena Fischer/)).toBeTruthy();
    expect(screen.queryByLabelText(/Margince/)).toBeNull();

    await userEvent.click(screen.getByLabelText(/Lena Fischer/));
    await userEvent.click(screen.getByLabelText(/Vertrieb/));
    await userEvent.click(
      screen.getByRole("button", { name: "Save visibility" }),
    );

    const write = sent.find(
      (each) => each.key === "PATCH /activities/a6/audience",
    );
    // Fails loudly rather than passing vacuously: `find` answering undefined
    // makes `write?.body` undefined, and a toEqual against undefined would
    // report the write as absent instead of wrong.
    if (!write) {
      throw new Error(
        `no audience write was sent; saw ${sent.map((e) => e.key).join(", ")}`,
      );
    }
    expect(write.body).toEqual({
      audience: "selected",
      members: [
        { subject_type: "user", subject_id: "u-2" },
        { subject_type: "team", subject_id: "t-1" },
      ],
    });
  });

  // Limiting a message to NOBODY is not a limit anybody meant, and it would
  // lock the author out of their own correspondence. The confirm refuses it
  // rather than the server having to.
  it("refuses to save a named audience with nobody named", async () => {
    stubRoutes({
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u-2",
              display_name: "Lena Fischer",
              email: "lena@demo.test",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /teams": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(
      <TimelineActions
        activity={{
          ...activity202,
          id: "a8",
          // A note, for the same reason as the test above: this dialog is the
          // row's, and an email row no longer draws one.
          kind: "note",
          captured_by: "human:u1",
          version: 3,
        }}
        entityType="deal"
        entityId="d1"
      />,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Change visibility" }),
    );
    await userEvent.click(screen.getByLabelText(/Named people/));
    await screen.findByLabelText(/Lena Fischer/);

    expect(
      screen
        .getByRole("button", { name: "Save visibility" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });

  // A withheld row offers nothing: somebody outside the audience has no
  // standing to change who else is in it. The kind that still draws the
  // control is the one that has to prove this.
  it("offers no audience control on a withheld row", async () => {
    stubRoutes();
    render(
      <TimelineActions
        activity={{
          ...activity202,
          id: "a9",
          kind: "note",
          content_state: "withheld",
        }}
        entityType="deal"
        entityId="d1"
      />,
    );
    await relinkSettled();
    expect(
      screen.queryByRole("button", { name: "Change visibility" }),
    ).toBeNull();
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
  });
});
