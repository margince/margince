// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import type { components } from "../../api/schema";
import { viewerZone } from "../../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { OpenEmailDrawer } from "./openemaildrawer";

// The record page's mail drawer, which composes the email detail with the
// filing line, the access line and the reply verb.

type EmailSummary = components["schemas"]["EmailSummary"];

const BASE: EmailSummary = {
  activity_id: "11111111-1111-4111-8111-111111111111",
  occurred_at: "2026-09-01T09:12:00Z",
  occurred_precision: "instant",
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

// The contact the sender resolves to and the account the message is filed
// against, named by the record reads the drawer's filing line makes.
const ANA = "22222222-2222-4222-8222-222222222222";
const BRANDT = "33333333-3333-4333-8333-333333333333";

const meta: Meta<typeof OpenEmailDrawer> = {
  title: "Records/Record 360/Open email drawer",
  component: OpenEmailDrawer,
};
export default meta;

/**
 * The open drawer; a closed one draws nothing. It shows the whole envelope:
 * the sender named as their contact, the filed records and the reply verb.
 * Each opens beside the message, so a reader who follows a contact keeps the
 * mail.
 */
export const RecordDrawer: StoryObj = {
  render: () => {
    installFetchStub({
      // The reader, which decides whether a relay send reads "for you".
      "GET /me": meRoute({}),
      [`GET /activities/${BASE.activity_id}`]: () =>
        jsonResponse({
          id: BASE.activity_id,
          content_state: "available",
          commitment_admissions: [
            {
              locator: "pricing",
              summary: "Send revised pricing",
              source_quote: "I will send revised pricing.",
              outcome: "historical_without_current_deadline",
              current_work: false,
              rule_version: "1",
              evidence_ids: [],
              evaluated_at: "2026-10-18T10:00:00Z",
            },
          ],
        }),
      "GET /activities/11111111-1111-4111-8111-111111111111/email-presentation":
        () =>
          jsonResponse({
            id: BASE.activity_id,
            lifecycle: "delivered",
            occurred_at: BASE.occurred_at,
            summary: BASE,
            body: "Können wir Dienstag kurz sprechen?\n\nViele Grüße\nAna",
            from: [
              {
                address: "ana@brandt.example",
                display_name: "Ana Sommer",
                contact_id: ANA,
              },
            ],
            to: [],
            cc: [],
            bcc: [],
            bcc_withheld: false,
            attachments: [],
            links: [
              { entity_type: "contact", entity_id: ANA },
              { entity_type: "company", entity_id: BRANDT },
            ],
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
      // `EntityRef` reads one record per link. Without these reads the
      // filing line would show ids instead of names.
      [`GET /contacts/${ANA}`]: () =>
        jsonResponse({ id: ANA, full_name: "Ana Sommer" }),
      [`GET /companies/${BRANDT}`]: () =>
        jsonResponse({ id: BRANDT, display_name: "Brandt Automotive" }),
    });
    return (
      <StoryProviders>
        {/* The reader's own zone: format/zone-by-purpose forbids naming one. */}
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
 * A held message: the access line shows the mark, the reason and the verb.
 * Without the reason, a reader cannot tell a correct verdict from a wrong one.
 * `can_change` is true, so the verb and its scope caption show too.
 */
export const RecordDrawerHeld: StoryObj = {
  render: () => {
    installFetchStub({
      // The reader, which decides whether a relay send reads "for you".
      "GET /me": meRoute({}),
      [`GET /activities/${BASE.activity_id}`]: () =>
        jsonResponse({
          id: BASE.activity_id,
          content_state: "available",
          commitment_admissions: [
            {
              locator: "pricing",
              summary: "Send revised pricing",
              source_quote: "I will send revised pricing.",
              outcome: "historical_without_current_deadline",
              current_work: false,
              rule_version: "1",
              evidence_ids: [],
              evaluated_at: "2026-10-18T10:00:00Z",
            },
          ],
        }),
      "GET /activities/11111111-1111-4111-8111-111111111111/email-presentation":
        () =>
          jsonResponse({
            id: BASE.activity_id,
            lifecycle: "delivered",
            occurred_at: BASE.occurred_at,
            summary: { ...BASE, display_status: "participants" },
            // Without a thread the server offers no `thread_contribution`,
            // and the control draws nothing.
            thread_key: "brandt-angebot-q4",
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
              change_scope: "thread",
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
