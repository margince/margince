/** @vitest-environment happy-dom */

// The email drawer's verbs: Reply and Forward open the composer in place of
// the drawer, and the drawer comes back when the composer closes.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { messageText } from "../../design-system/richtext-testing";
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

const PRESENTATION: EmailPresentation = {
  id: MESSAGE,
  lifecycle: "delivered",
  occurred_at: "2026-09-01T09:15:00Z",
  summary: {
    activity_id: MESSAGE,
    occurred_at: "2026-09-01T09:15:00Z",
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

function drawer() {
  stubRoutes({
    [`GET /activities/${MESSAGE}/email-presentation`]: () =>
      jsonResponse(PRESENTATION),
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
  });
  render(
    <OpenEmailDrawer
      activityId={MESSAGE}
      zone={viewerZone()}
      onClose={vi.fn()}
    />,
  );
}

describe("the email drawer's verbs", () => {
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
