// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { viewerZone } from "../../format/timezone";
import {
  dropsOperatorReason,
  editSeedOf,
  isEditable,
  unavailableFilesOf,
} from "./schedulededit";
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

it("seeds the markup, the files and the draft reference the message was frozen with", () => {
  const seed = editSeedOf({
    ...SEND,
    html_body:
      '<p>One <strong>bold</strong> <a href="https://acme.test">link</a></p>',
    attachments: [
      {
        id: "file-1",
        filename: "quote.pdf",
        content_type: "application/pdf",
        byte_size: 2048,
      },
    ],
    draft_ref: "draft-ref-1",
  });
  expect(seed).toMatchObject({
    html: '<p>One <strong>bold</strong> <a href="https://acme.test">link</a></p>',
    files: [{ id: "file-1", filename: "quote.pdf", byteSize: 2048 }],
    draftRef: "draft-ref-1",
  });
});

it("draws a plain message's paragraphs and seeds no files and no draft", () => {
  expect(editSeedOf(SEND)).toMatchObject({
    html: "<p>One</p><p>Two</p>",
    files: [],
    draftRef: "",
  });
});

it("also edits a held message, which a human has to act on", () => {
  expect(editSeedOf({ ...SEND, status: "held" })).toBeDefined();
});

it.each([
  ["has gone", { status: "sent" as const }],
  ["was withdrawn", { status: "cancelled" as const }],
  ["names no record", { links: [] }],
  ["claims a marketing purpose", { marketing_purpose: "marketing_email" }],
  ["carries the legacy consent key", { consent_purpose: "marketing_email" }],
  ["carries an operator reason", { operator_reason: "Asked at the fair" }],
  ["holds a file the send would refuse", { attachments_unavailable: 1 }],
])("offers no edit on a message that %s", (_, change) => {
  expect(editSeedOf({ ...SEND, ...change })).toBeUndefined();
});

it("seeds a reply from the thread's links and names the email it answers", () => {
  const reply = { ...SEND, anchor_activity_id: "activity-1", links: undefined };
  expect(isEditable(reply)).toBe(true);
  expect(editSeedOf(reply)).toBeUndefined();
  expect(
    editSeedOf(reply, [
      { entity_type: "company", entity_id: "company-9" },
      { entity_type: "contact", entity_id: "contact-9" },
    ]),
  ).toMatchObject({
    replyTo: "activity-1",
    anchor: { type: "company", id: "company-9" },
    recipientId: "contact-9",
  });
});

it("refuses the edit for an operator reason only where nothing else stops it", () => {
  const reasoned = { ...SEND, operator_reason: "Asked at the fair" };
  expect(isEditable(reasoned)).toBe(false);
  expect(dropsOperatorReason(reasoned)).toBe(true);
  expect(dropsOperatorReason(SEND)).toBe(false);
  expect(
    dropsOperatorReason({ ...reasoned, marketing_purpose: "marketing_email" }),
  ).toBe(false);
  expect(dropsOperatorReason({ ...reasoned, status: "sent" })).toBe(false);
});

it("counts the files the send would refuse, none when the read names none", () => {
  expect(unavailableFilesOf(SEND)).toBe(0);
  expect(unavailableFilesOf({ ...SEND, attachments_unavailable: 2 })).toBe(2);
});
