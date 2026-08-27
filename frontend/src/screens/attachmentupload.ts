// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { throwProblem } from "./common";

// Putting bytes on a record, for every surface that does it: the document
// dialog's library upload and the contract form's signed paper.
//
// IT SENDS MULTIPART BY HAND because the generated client only serializes JSON
// and this endpoint takes a file part. Everything else about the request — the
// cookie, the problem-document shape of a refusal — is unchanged, which is the
// whole reason this is a thin wrapper and not a second client.

type Attachment = components["schemas"]["Attachment"];

/**
 * The record bytes can be filed against. The union is the ENDPOINT's, so a
 * surface that files a document states its parent in the endpoint's own terms
 * rather than keeping a private list that can fall behind it.
 */
export type AttachmentParent = Readonly<{
  entityType: "organization" | "person" | "deal";
  entityId: string;
}>;

/**
 * uploadAttachment stores one file against one parent.
 *
 * `extraParts` is what a particular FILING adds beyond the parent and the
 * bytes — a `contract_id` when the document is an agreement's signed paper, and
 * nothing at all when it is the record's own library. They are appended before
 * the file, so the bytes stay the last part of the body as they were.
 *
 * Returns the stored document, or undefined when the server accepted the bytes
 * and the response body could not be read. Those are different facts and a
 * caller may need both: the second one is a stored document whose id we do not
 * know, so its metadata cannot be written — but reporting it as a failed upload
 * would be a lie about a file that is on the record.
 */
export async function uploadAttachment(
  parent: AttachmentParent,
  file: File,
  extraParts: Readonly<Record<string, string>> = {},
): Promise<Attachment | undefined> {
  const body = new FormData();
  body.append("entity_type", parent.entityType);
  body.append("entity_id", parent.entityId);
  for (const [part, value] of Object.entries(extraParts)) {
    body.append(part, value);
  }
  body.append("file", file);
  // contract-fetch:allow multipart — the generated client serializes JSON only, and this operation takes a file part
  const response = await fetch("/v1/attachments", {
    method: "POST",
    body,
    credentials: "include",
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => undefined);
    throwProblem(payload);
  }
  // Not thrown. Past the status check the bytes are stored, and nothing a
  // failed parse tells us changes that.
  const stored: Attachment | undefined = await response
    .json()
    .catch(() => undefined);
  return stored;
}
