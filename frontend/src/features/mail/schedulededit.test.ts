// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { viewerZone } from "../../format/timezone";
import { editSeedOf } from "./schedulededit";
import type { ScheduledSend } from "./scheduledqueue";

const SEND: ScheduledSend = {
  id: "send-1",
  status: "scheduled",
  scheduled_at: "2026-09-01T07:00:00Z",
  scheduled_tz: viewerZone(),
  subject: "The quote",
  to: ["ceo@acme.test"],
  bcc: ["boss@acme.test"],
  body: "One\n\nTwo",
  links: [
    { entity_type: "company", entity_id: "company-1" },
    { entity_type: "contact", entity_id: "contact-1" },
    { entity_type: "deal", entity_id: "deal-1" },
  ],
  version: 1,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
};

it("seeds the composer from the first link and the grounding links after it", () => {
  expect(editSeedOf(SEND)).toMatchObject({
    anchor: { type: "company", id: "company-1" },
    bcc: ["boss@acme.test"],
    recipientId: "contact-1",
    dealId: "deal-1",
    projectId: "",
  });
});

it("also edits a held message, which a human has to act on", () => {
  expect(editSeedOf({ ...SEND, status: "held" })).toBeDefined();
});

it.each([
  ["has gone", { status: "sent" as const }],
  ["was withdrawn", { status: "cancelled" as const }],
  ["answers a thread", { anchor_activity_id: "activity-1" }],
  ["names no record", { links: [] }],
  ["claims a marketing purpose", { marketing_purpose: "marketing_email" }],
  ["carries the legacy consent key", { consent_purpose: "marketing_email" }],
])("offers no edit on a message that %s", (_, change) => {
  expect(editSeedOf({ ...SEND, ...change })).toBeUndefined();
});
