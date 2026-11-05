// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { postMultipart } from "./multipart";

// Putting bytes on a record, for every surface that does it: the document
// dialog's library upload and the contract form's signed paper.
//
// IT SENDS MULTIPART BY HAND because the generated client only serializes JSON
// and this endpoint takes a file part. Everything else about the request — the
// cookie, the problem-document shape of a refusal — is unchanged, which is the
// whole reason this is a thin wrapper and not a second client.

type Attachment = components["schemas"]["Attachment"];

/**
 * The extensions the server accepts, mirroring the activities table and held
 * to it by `backend/gates/attachmenttypes_test.go`. A drop bypasses the
 * picker's filter, so the server's refusal stays the authority.
 */
export const ACCEPTED_ATTACHMENT_EXTENSIONS = [
  ".pdf",
  ".doc",
  ".docx",
  ".xls",
  ".xlsx",
  ".ppt",
  ".pptx",
  ".odt",
  ".ods",
  ".odp",
  ".rtf",
  ".txt",
  ".csv",
  ".md",
  ".png",
  ".jpg",
  ".jpeg",
  ".gif",
  ".webp",
  ".heic",
  ".heif",
  ".tif",
  ".tiff",
  ".html",
  ".htm",
  ".zip",
  ".eml",
  ".msg",
] as const;

/** The same list as a file input's `accept` attribute. */
export const ACCEPTED_ATTACHMENT_ATTR =
  ACCEPTED_ATTACHMENT_EXTENSIONS.join(",");

/**
 * The record bytes can be filed against. The union is the ENDPOINT's, so a
 * surface that files a document states its parent in the endpoint's own terms
 * rather than keeping a private list that can fall behind it.
 */
export type AttachmentParent = Readonly<{
  entityType: "activity" | "company" | "contact" | "deal" | "lead" | "project";
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
  const stored = await postMultipart("/v1/attachments", {
    entity_type: parent.entityType,
    entity_id: parent.entityId,
    ...extraParts,
    file,
  });
  return stored as Attachment | undefined;
}
