// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { throwProblem } from "../../lib/problem";
import type { RelinkKind } from "./composerelink";

// The saved draft's calls to the server, and the anchor and cache key they
// share. The hook that decides when to make them is composesaveddraft.tsx.

export type MailDraft = components["schemas"]["MailDraft"];
type MailDraftAnchorType = components["schemas"]["MailDraftAnchorType"];

export type SavedDraftAnchor = Readonly<{
  type: MailDraftAnchorType;
  id: string;
}>;

/** The composer's fields as a draft keeps them. */
export type SavedDraftFields = Readonly<{
  to: readonly string[];
  cc: readonly string[];
  bcc: readonly string[];
  subject: string;
  body: string;
  html: string;
  // Where an agent drafted it, so a restore files the send there rather than
  // wherever the composer was opened.
  //
  // Absent rather than empty on the way OUT: the composer saving a draft has
  // no grounding to report, and only a restore carries one.
  grounding?: readonly { readonly type: string; readonly id: string }[];
}>;

/**
 * The filing a restored draft starts from: the deal and project it was
 * written against.
 *
 * Only these two, because the contact comes from the recipient and the other
 * anchor kinds are not filings a composer can set. A draft grounded in
 * several of one kind keeps the first, since the composer holds one.
 */
export function groundedFiling(grounding: SavedDraftFields["grounding"]): {
  dealId: string;
  projectId: string;
} {
  const first = (kind: string) =>
    (grounding ?? []).find((ground) => ground.type === kind)?.id ?? "";
  return { dealId: first("deal"), projectId: first("project") };
}

// A reply keeps its draft on the message it answers, a new message on the
// record it was started from. A channel reply keeps none: the draft is a mail's
// fields, and a chat message has no subject or addressee to hold.
export function savedDraftAnchor(input: {
  answering: string | undefined;
  entityType: RelinkKind;
  entityId: string;
  isChannelReply: boolean;
}): SavedDraftAnchor | null {
  if (input.isChannelReply) return null;
  if (input.answering) return { type: "activity", id: input.answering };
  return input.entityId ? { type: input.entityType, id: input.entityId } : null;
}

export function draftKey(anchor: SavedDraftAnchor | null) {
  return ["mail-draft", anchor?.type ?? "", anchor?.id ?? ""] as const;
}

// A 404 is the ordinary answer. It means no draft here, or an anchor this
// reader can no longer see; the server does not tell the two apart.
export async function readDraft(
  anchor: SavedDraftAnchor,
): Promise<MailDraft | null> {
  const { data, error, response } = await api.GET("/mail-drafts", {
    params: { query: { anchor_type: anchor.type, anchor_id: anchor.id } },
  });
  if (response.status === 404) return null;
  if (!response.ok) throwProblem(error || {});
  return isMailDraft(data) ? data : null;
}

// A 200 is read as a draft only when it has a draft's shape. An answer that is
// not one (a proxy's page, a fixture's catch-all) restores nothing.
function isMailDraft(value: MailDraft | undefined): value is MailDraft {
  return (
    typeof value?.id === "string" &&
    typeof value.body === "string" &&
    typeof value.subject === "string" &&
    Array.isArray(value.to) &&
    Array.isArray(value.cc) &&
    Array.isArray(value.bcc)
  );
}

export async function deleteDraft(id: string): Promise<void> {
  const { error, response } = await api.DELETE("/mail-drafts/{id}", {
    params: { path: { id } },
  });
  // Already gone is the outcome the reader asked for.
  if (!response.ok && response.status !== 404) {
    throwProblem(error || {});
  }
}

export type SaveAsk = Readonly<{
  anchor: SavedDraftAnchor;
  fields: SavedDraftFields;
  version: number | undefined;
  /** Which send this save was asked under; a later send retires it. */
  sends: number;
  /** Kept as the reader writes: no toast, and the window stays open. */
  quiet?: boolean;
}>;

export async function writeDraft(ask: SaveAsk): Promise<MailDraft> {
  const { data, error, response } = await api.PUT("/mail-drafts", {
    // No version is a create: the server refuses it 409 when a draft already
    // exists here, so an unpinned write can never replace one unseen.
    params: { ...(ask.version === undefined ? {} : ifMatch(ask.version)) },
    body: {
      anchor_type: ask.anchor.type,
      anchor_id: ask.anchor.id,
      to: [...ask.fields.to],
      cc: [...ask.fields.cc],
      bcc: [...ask.fields.bcc],
      subject: ask.fields.subject,
      body: ask.fields.body,
      html_body: ask.fields.html.trim() === "" ? null : ask.fields.html,
    },
  });
  if (!response.ok || !isMailDraft(data)) {
    throwProblem(error || {});
  }
  return data;
}
