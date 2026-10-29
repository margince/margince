/** @vitest-environment happy-dom */

// The email drawer's verbs: Reply and Forward open the composer in place of
// the drawer, and the drawer comes back when the composer closes.

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { threadOrder } from "../../design-system/emailthread";
import {
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { viewerZone } from "../../format/timezone";
import {
  activity202,
  jsonResponse,
  render,
  stubRoutes,
} from "./compose.testkit";
import { OpenEmailDrawer } from "./openemaildrawer";

type EmailPresentation = components["schemas"]["EmailPresentation"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const MESSAGE = "01a05500-0000-7000-8000-0000000000a1";
const ANA = "01a05500-0000-7000-8000-0000000000c1";
const LATEST = "01a05500-0000-7000-8000-0000000000a2";

const PRESENTATION: EmailPresentation = {
  id: MESSAGE,
  lifecycle: "delivered",
  occurred_at: "2026-09-01T09:15:00Z",
  occurred_precision: "instant",
  summary: {
    activity_id: MESSAGE,
    occurred_at: "2026-09-01T09:15:00Z",
    occurred_precision: "instant",
    version: 3,
    subject: "Angebot Q4",
    preview: "Können wir Dienstag kurz sprechen?",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 0,
  },
  body: "Können wir Dienstag kurz sprechen?",
  thread_key: "t1",
  from: [{ address: "ana@brandt.example", display_name: "Ana Sommer" }],
  to: [{ address: "lars@margince.example", display_name: null }],
  cc: [],
  bcc: [],
  bcc_withheld: false,
  attachments: [],
  links: [{ entity_type: "contact", entity_id: ANA }],
  thread: { members: [], next_cursor: null },
  access: {
    content_state: "available",
    display_status: "team",
    can_change: false,
    change_mode: "none",
  },
  can_reply: true,
  can_relink: false,
  version: 3,
};

function drawer(
  initial = PRESENTATION,
  overrides: Record<string, () => Response> = {},
) {
  const sent = stubRoutes({
    [`GET /activities/${MESSAGE}/email-presentation`]: () =>
      jsonResponse(initial),
    // The composer keeps its words as a draft; the server echoes them back.
    "PUT /mail-drafts": () =>
      jsonResponse({
        id: "md-1",
        anchor_type: "contact",
        anchor_id: ANA,
        to: [],
        cc: [],
        bcc: [],
        subject: "",
        body: "",
        html_body: null,
        version: 1,
        agent_drafted: false,
        created_at: "2026-09-01T09:20:00Z",
        updated_at: "2026-09-01T09:20:00Z",
      }),
    // The reply composer reads the message it answers.
    [`GET /activities/${MESSAGE}`]: () =>
      jsonResponse({
        ...activity202,
        id: MESSAGE,
        subject: "Angebot Q4",
        direction: "inbound",
      }),
    ...overrides,
  });
  render(
    <OpenEmailDrawer
      activityId={MESSAGE}
      zone={viewerZone()}
      onClose={vi.fn()}
    />,
  );
  return sent;
}

describe("the email drawer's verbs", () => {
  it("orders equal-time messages consistently whichever message was opened", () => {
    const sibling = { ...PRESENTATION.summary, activity_id: LATEST };
    const fromOlder = threadOrder({
      ...PRESENTATION,
      thread: { members: [sibling], next_cursor: null },
    });
    const fromNewer = threadOrder({
      ...PRESENTATION,
      id: LATEST,
      summary: sibling,
      thread: { members: [PRESENTATION.summary], next_cursor: null },
    });
    expect(fromOlder.map((message) => message.activity_id)).toEqual([
      LATEST,
      MESSAGE,
    ]);
    expect(fromNewer).toEqual(fromOlder);
  });

  it("keeps a newer withheld message and its actions private", async () => {
    const user = userEvent.setup();
    const newest = {
      ...PRESENTATION.summary,
      activity_id: LATEST,
      occurred_at: "2026-09-02T09:15:00Z",
    };
    drawer(
      { ...PRESENTATION, thread: { members: [newest], next_cursor: null } },
      {
        [`GET /activities/${LATEST}/email-presentation`]: () =>
          jsonResponse({
            ...PRESENTATION,
            id: LATEST,
            body: "Private newer text",
            can_reply: true,
            access: { ...PRESENTATION.access, content_state: "withheld" },
          }),
      },
    );
    expect(
      await screen.findByText("This message is not shared with you"),
    ).toBeTruthy();
    expect(screen.queryByText("Private newer text")).toBeNull();
    const newestRow = within(screen.getByRole("list")).getAllByRole(
      "listitem",
    )[0];
    expect(newestRow).toBeDefined();
    if (!newestRow) throw new Error("Newest message is missing");
    expect(
      within(newestRow).queryByRole("button", { name: "Reply" }),
    ).toBeNull();
    expect(
      within(newestRow).queryByRole("button", { name: "Forward" }),
    ).toBeNull();
    await user.click(
      within(newestRow).getByRole("button", { name: "Collapse" }),
    );
    expect(
      screen.queryByText("This message is not shared with you"),
    ).toBeNull();
  });

  it.each(["newest", "older"])(
    "sends a reply to the %s message from that message's actions",
    async (target) => {
      const user = userEvent.setup();
      const latest: EmailPresentation = {
        ...PRESENTATION,
        id: LATEST,
        occurred_at: "2026-09-02T09:15:00Z",
        summary: {
          ...PRESENTATION.summary,
          activity_id: LATEST,
          occurred_at: "2026-09-02T09:15:00Z",
          subject: "Updated offer",
        },
        body: "The revised offer works for us.",
        from: [
          { address: "buyer@example.test", display_name: "Latest sender" },
        ],
      };
      const sent = drawer(
        {
          ...PRESENTATION,
          thread: {
            members: [PRESENTATION.summary, latest.summary],
            next_cursor: null,
          },
        },
        {
          [`GET /activities/${LATEST}/email-presentation`]: () =>
            jsonResponse(latest),
          [`GET /activities/${LATEST}`]: () =>
            jsonResponse({
              ...activity202,
              id: LATEST,
              subject: "Updated offer",
              direction: "inbound",
            }),
          [`POST /activities/${LATEST}/send-email`]: () =>
            jsonResponse(activity202, 202),
          [`POST /activities/${MESSAGE}/send-email`]: () =>
            jsonResponse(activity202, 202),
        },
      );
      await screen.findByText(latest.body ?? "");
      const rows = within(screen.getByRole("list")).getAllByRole("listitem");
      expect(rows).toHaveLength(2);
      expect(rows[0].textContent).toContain("The revised offer works for us.");
      expect(rows[1].textContent).toContain(
        "Können wir Dienstag kurz sprechen?",
      );

      expect(screen.getByRole("article", { name: "Ana Sommer" })).toBeTruthy();
      const message = screen.getByRole("article", {
        name: target === "older" ? "Ana Sommer" : "Latest sender",
      });
      await user.click(within(message).getByRole("button", { name: "Reply" }));
      await screen.findByRole("dialog", {
        name: target === "older" ? "Re: Angebot Q4" : "Re: Updated offer",
      });
      await user.type(screen.getByLabelText("To"), "buyer@example.test");
      await user.tab();
      writeMessage("Body", "Let us pick this up next week.");
      await user.click(screen.getByRole("button", { name: "Send" }));
      const repliedTo = target === "older" ? MESSAGE : LATEST;
      await waitFor(() =>
        expect(
          sent.some(
            (request) =>
              request.key === `POST /activities/${repliedTo}/send-email`,
          ),
        ).toBe(true),
      );
      expect(
        await screen.findByText("Sent and added to this thread."),
      ).toBeTruthy();
    },
  );

  it("steps aside for a reply in the thread, and comes back when it closes", async () => {
    const user = userEvent.setup();
    drawer();

    await user.click(await screen.findByRole("button", { name: "Reply" }));

    expect(
      await screen.findByRole("dialog", { name: "Re: Angebot Q4" }),
    ).toBeTruthy();
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Angebot Q4" })).toBeNull(),
    );

    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(
      await screen.findByRole("dialog", { name: "Angebot Q4" }),
    ).toBeTruthy();
  });

  it("forwards the message as a new email carrying its words", async () => {
    const user = userEvent.setup();
    drawer();

    await user.click(await screen.findByRole("button", { name: "Forward" }));

    expect(await screen.findByDisplayValue("Fwd: Angebot Q4")).toBeTruthy();
    // The forwarded words sit under a header, below where the reader writes.
    await waitFor(() =>
      expect(messageText("Body")).toContain("Forwarded message"),
    );
    expect(messageText("Body")).toContain("Können wir Dienstag kurz sprechen?");
  });
});
