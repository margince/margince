// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../screens/story-utils";
import { Card } from "./atoms";
import { EmailDetail } from "./emaildetail";
import { EmailEntry } from "./emailentry";
import { EmailReference } from "./emailreference";
import { OpenEmailDrawer } from "./openemaildrawer";
import { Panel, PanelBody } from "./panel";

// The one email row, and the citation that opens the same drawer.
//
// The states worth a picture are the access ones, because they are what a
// reader has to tell apart at a glance: a message the team may read, one
// limited to the people on it, one narrowed to named colleagues, and one this
// reader may not read at all — which keeps its shape and loses its words.
//
// `InEveryHost` is the load-bearing one. The same row is drawn inside a
// timeline list item, a Panel and a Card, and it has to look identical in all
// three: that is the whole claim of "one row", and a picture falsifies it
// faster than a test can.

type EmailSummary = components["schemas"]["EmailSummary"];

const BASE: EmailSummary = {
  activity_id: "11111111-1111-4111-8111-111111111111",
  occurred_at: "2026-09-01T09:12:00Z",
  display_status: "team",
  attachment_count: 0,
  move: "needs_reply",
  version: 3,
  subject: "Angebot Q4 — Rückfragen zur Laufzeit",
  preview:
    "Können wir Dienstag kurz sprechen? Die Laufzeit passt uns, beim Preis müssten wir nachverhandeln.",
  direction: "inbound",
  counterparty: "Ana Sommer",
};

function Row({ summary }: Readonly<{ summary: EmailSummary }>) {
  return (
    <EmailEntry summary={summary} timestamp="1 Sep 09:12" onOpen={() => {}} />
  );
}

const meta: Meta<typeof Row> = {
  title: "Design System/Email entry",
  component: Row,
};
export default meta;

type Story = StoryObj<typeof Row>;

/** Readable by anyone who can reach a linked record. */
export const Team: Story = { args: { summary: BASE } };

/** Limited to the people on the correspondence. */
export const Participants: Story = {
  args: { summary: { ...BASE, display_status: "participants" } },
};

/** Narrowed to named colleagues besides the participants. */
export const Selected: Story = {
  args: {
    summary: { ...BASE, display_status: "selected", attachment_count: 3 },
  },
};

/**
 * Outside the audience. The row keeps its shape — a reader can tell a limited
 * conversation from one that never happened — and carries none of its words.
 */
export const Withheld: Story = {
  args: {
    summary: {
      ...BASE,
      display_status: "withheld",
      subject: undefined,
      preview: undefined,
      counterparty: undefined,
      move: "none",
    },
  },
};

/** A message with no subject of its own says so, rather than drawing a gap. */
export const NoSubject: Story = {
  args: { summary: { ...BASE, subject: undefined } },
};

/** Nothing the sender wrote: a forward carrying no words of its own. */
export const NoPreview: Story = {
  args: { summary: { ...BASE, preview: undefined, move: "waiting_for_them" } },
};

/** A long subject truncates rather than pushing the timestamp off the line. */
export const LongSubject: Story = {
  args: {
    summary: {
      ...BASE,
      subject:
        "Angebot Q4 — Rückfragen zur Laufzeit, zum Wartungsfenster und zu den Abnahmekriterien der zweiten Phase",
      counterparty: "Ana Sommer +4",
      attachment_count: 12,
    },
  },
};

/**
 * The claim: one row, three hosts. A timeline item, a Panel and a Card each
 * supply position and nothing else, so the row reads the same in all of them.
 */
export const InEveryHost: StoryObj = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-4)", maxWidth: 640 }}>
      <ul style={{ margin: 0, padding: 0, listStyle: "none" }}>
        <li>
          <EmailEntry
            summary={BASE}
            timestamp="1 Sep 09:12"
            onOpen={() => {}}
          />
        </li>
      </ul>
      <Panel title="Recent">
        <PanelBody>
          <EmailEntry
            summary={BASE}
            timestamp="1 Sep 09:12"
            onOpen={() => {}}
          />
        </PanelBody>
      </Panel>
      <Card>
        <EmailEntry summary={BASE} timestamp="1 Sep 09:12" onOpen={() => {}} />
      </Card>
    </div>
  ),
};

/**
 * The citation form: subject and date, no preview and no badge, opening the
 * same drawer a row opens. The second has no subject and no opener.
 */
export const Reference: StoryObj = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-2)" }}>
      <EmailReference
        subject="Angebot Q4 — Rückfragen zur Laufzeit"
        occurredAt="1 Sep"
        onOpen={() => {}}
      />
      <EmailReference subject={null} occurredAt="28 Aug" />
    </div>
  ),
};

/**
 * The drawer a row opens: the message whole, its recipients, and the sign-off
 * folded behind a control rather than dropped.
 */
export const Detail: StoryObj = {
  render: () => {
    installFetchStub({
      "GET /activities/11111111-1111-4111-8111-111111111111/email-presentation":
        () =>
          jsonResponse({
            id: BASE.activity_id,
            lifecycle: "delivered",
            occurred_at: BASE.occurred_at,
            summary: BASE,
            body: "Können wir Dienstag kurz sprechen? Die Laufzeit passt uns.\n\nViele Grüße\nAna Sommer\nBrandt Automotive",
            from: [
              { address: "ana@brandt.example", display_name: "Ana Sommer" },
            ],
            to: [{ address: "lars@margince.example", display_name: "Lars J." }],
            cc: [],
            bcc: [],
            bcc_withheld: true,
            attachments: [],
            links: [],
            access: {
              content_state: "available",
              display_status: "team",
              can_change: false,
              change_mode: "none",
            },
            can_reply: true,
            can_relink: false,
            version: 3,
          }),
    });
    return (
      <StoryProviders>
        <EmailDetail
          activityId={BASE.activity_id}
          onClose={() => {}}
          formatWhen={() => "1 Sep 09:12"}
        />
      </StoryProviders>
    );
  },
};

/**
 * The record page's drawer, which draws nothing when no message is open. The
 * story shows the open state; the closed one is an empty canvas by design.
 */
export const RecordDrawer: StoryObj = {
  render: () => {
    installFetchStub({
      "GET /activities/11111111-1111-4111-8111-111111111111/email-presentation":
        () =>
          jsonResponse({
            id: BASE.activity_id,
            lifecycle: "delivered",
            occurred_at: BASE.occurred_at,
            summary: BASE,
            body: "Können wir Dienstag kurz sprechen?\n\nViele Grüße\nAna",
            from: [
              { address: "ana@brandt.example", display_name: "Ana Sommer" },
            ],
            to: [],
            cc: [],
            bcc: [],
            bcc_withheld: false,
            attachments: [],
            links: [],
            access: {
              content_state: "available",
              display_status: "team",
              can_change: false,
              change_mode: "none",
            },
            can_reply: true,
            can_relink: false,
            version: 3,
          }),
    });
    return (
      <StoryProviders>
        {/* The reader's own zone, as every record page passes its own: a
            story that pinned one would be the only place in the tree naming a
            timezone, which is what format/zone-by-purpose holds. */}
        <OpenEmailDrawer
          activityId={BASE.activity_id}
          zone={viewerZone()}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};

/**
 * The drawer for a message something HELD, which is the state the access
 * markers exist for.
 *
 * Two badges beside the subject: what the limit is, and what decided it. The
 * pair is the picture worth having, because a header carrying only "Participants"
 * tells a reader they cannot widen the message and not why — and a reader who
 * cannot see the verdict cannot tell a correct one from a wrong one.
 *
 * `can_change` is true here, so the sentence and the control under the body are
 * drawn too: this is the whole arrangement, header and footer, in one shot.
 */
export const RecordDrawerHeld: StoryObj = {
  render: () => {
    installFetchStub({
      "GET /activities/11111111-1111-4111-8111-111111111111/email-presentation":
        () =>
          jsonResponse({
            id: BASE.activity_id,
            lifecycle: "delivered",
            occurred_at: BASE.occurred_at,
            summary: { ...BASE, display_status: "participants" },
            body: "Können wir Dienstag kurz sprechen?\n\nViele Grüße\nAna",
            from: [
              { address: "ana@brandt.example", display_name: "Ana Sommer" },
            ],
            to: [],
            cc: [],
            bcc: [],
            bcc_withheld: false,
            attachments: [],
            links: [],
            access: {
              content_state: "available",
              display_status: "participants",
              audience: "participants",
              explanation: "explicitly_confidential",
              can_change: true,
              change_mode: "thread_contribution",
            },
            can_reply: true,
            can_relink: false,
            version: 3,
          }),
    });
    return (
      <StoryProviders>
        <OpenEmailDrawer
          activityId={BASE.activity_id}
          zone={viewerZone()}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};
