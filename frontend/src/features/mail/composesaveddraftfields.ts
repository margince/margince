// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { paragraphsFrom } from "../../design-system/richtext";
import { localDateTimeValue } from "../../format/calendarday";
import { stable } from "../../format/collate";
import type { ChosenFile } from "./composeattachments";
import type { Grounding } from "./composedraftcall";
import type { ComposeFields } from "./composefields";
import {
  groundedFiling,
  type MailDraft,
  type SavedDraftFields,
} from "./composesaveddraftcalls";

/** What the composer holds that a saved draft keeps. */
export function savedFieldsOf(
  fields: ComposeFields,
  files: readonly ChosenFile[],
  filing: Partial<Grounding> | null,
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
    editorState: {
      ...fields.editorState,
      filing: selectedFiling(filing),
    },
  };
}

function selectedFiling(filing: Partial<Grounding> | null) {
  const selection: components["schemas"]["MailDraftGround"][] = [];
  if (filing?.dealId)
    selection.push({ anchor_type: "deal", anchor_id: filing.dealId });
  if (filing?.projectId)
    selection.push({ anchor_type: "project", anchor_id: filing.projectId });
  return selection;
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
    editorState: draft.editor_state,
    grounding: (draft.grounding ?? []).map((ground) => ({
      type: ground.anchor_type,
      id: ground.anchor_id,
    })),
  };
}

function sameList(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((value, index) => value === b[index]);
}

const filingKeys = (fields: SavedDraftFields) =>
  (
    fields.editorState?.filing ??
    selectedFiling(groundedFiling(fields.grounding))
  )
    .map((ground) => `${ground.anchor_type}:${ground.anchor_id}`)
    .sort(stable);

function sameEditorState(
  a: SavedDraftFields["editorState"],
  b: SavedDraftFields["editorState"],
) {
  return (
    (a?.intent ?? "") === (b?.intent ?? "") &&
    (a?.ai_generated ?? false) === (b?.ai_generated ?? false) &&
    (a?.ai_disclosure ?? "") === (b?.ai_disclosure ?? "") &&
    a?.voice_profile_version === b?.voice_profile_version &&
    (a?.voice_degraded ?? false) === (b?.voice_degraded ?? false) &&
    (a?.served_body ?? "") === (b?.served_body ?? "")
  );
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
    a.sendAt === b.sendAt &&
    sameList(filingKeys(a), filingKeys(b)) &&
    sameEditorState(a.editorState, b.editorState)
  );
}
