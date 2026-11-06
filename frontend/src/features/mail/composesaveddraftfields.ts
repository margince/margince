// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { paragraphsFrom } from "../../design-system/richtext";
import { localDateTimeValue } from "../../format/calendarday";
import type { ChosenFile } from "./composeattachments";
import type { ComposeFields } from "./composefields";
import type { MailDraft, SavedDraftFields } from "./composesaveddraftcalls";

/** What the composer holds that a saved draft keeps. */
export function savedFieldsOf(
  fields: ComposeFields,
  files: readonly ChosenFile[],
): SavedDraftFields {
  return {
    to: fields.to,
    cc: fields.cc,
    bcc: fields.bcc,
    subject: fields.subject,
    body: fields.body,
    html: fields.html,
    files,
    context: fields.context,
    sendAt: fields.sendAt,
  };
}

/** What a saved draft puts back in the composer. */
export function fieldsOfDraft(draft: MailDraft): SavedDraftFields {
  return {
    to: draft.to,
    cc: draft.cc,
    bcc: draft.bcc,
    subject: draft.subject,
    body: draft.body,
    html: draft.html_body ?? paragraphsFrom(draft.body),
    files: (draft.attachments ?? []).map((file) => ({
      id: file.id,
      filename: file.filename,
      byteSize: file.byte_size,
    })),
    context: draft.communication_context ?? "",
    sendAt: draft.scheduled_at ? localDateTimeValue(draft.scheduled_at) : "",
    grounding: (draft.grounding ?? []).map((ground) => ({
      type: ground.anchor_type,
      id: ground.anchor_id,
    })),
  };
}

function sameList(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((value, index) => value === b[index]);
}

const idsOf = (files: readonly ChosenFile[]) => files.map((file) => file.id);

// Mirrors the server's canonical address (`canonicalAddresses`, activities):
// trimmed, lower case, blanks dropped.
const canonical = (line: readonly string[]) =>
  line
    .map((address) => address.trim().toLowerCase())
    .filter((address) => address !== "");

const sameLine = (a: readonly string[], b: readonly string[]) =>
  sameList(canonical(a), canonical(b));

export function sameFields(a: SavedDraftFields, b: SavedDraftFields): boolean {
  return (
    sameLine(a.to, b.to) &&
    sameLine(a.cc, b.cc) &&
    sameLine(a.bcc, b.bcc) &&
    a.subject === b.subject &&
    a.body === b.body &&
    a.html === b.html &&
    sameList(idsOf(a.files), idsOf(b.files)) &&
    a.context === b.context &&
    a.sendAt === b.sendAt
  );
}
