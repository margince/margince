// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryKey } from "@tanstack/react-query";
import type { components } from "../api/schema";

// Which cached file lists show a file, for every write that adds, moves or
// removes one. The server's rule is `documentMembership` in
// backend/internal/modules/activities/documentmembership.go:
//
//   - a contact lists its own uploads and the files of every message linked to it;
//   - a deal lists its own uploads and the files of every message linked to it;
//   - a company lists every upload stamped with it (its own and its deals') and
//     the files of every message filed under it;
//   - a deal room shares files out of its deal's list.
//
// So a file's parent decides which lists hold it. A message's links are not
// known where its file is written, so those lists refresh by prefix. Only the
// lists the reader has open refetch; the others are marked stale.

type Attachment = components["schemas"]["Attachment"];

const ACTIVITY_FILES = ["attachments", "activity"] as const;
const CONTACT_FILES = ["attachments", "contact"] as const;
const DEAL_FILES = ["deal-documents"] as const;
const COMPANY_DOCUMENTS = ["companyDocuments"] as const;
const ROOM_DOCUMENTS = ["deal-room-documents"] as const;

/** One activity's own file list. */
export const activityFilesKey = (activityId: string) =>
  [...ACTIVITY_FILES, activityId] as const;

/** A contact's Documents list, which the tab strip's count reads too. */
export const contactFilesKey = (contactId: string) =>
  [...CONTACT_FILES, contactId] as const;

/** A deal's Files area, with and without hidden rows below it. */
export const dealFilesKey = (dealId: string) =>
  [...DEAL_FILES, dealId] as const;

/** A company's document library. */
export const companyDocumentsKey = (companyId: string) =>
  [...COMPANY_DOCUMENTS, companyId] as const;

/** One deal room's shared documents. */
export const roomDocumentsKey = (roomId: string) =>
  [...ROOM_DOCUMENTS, roomId] as const;

/**
 * Where a file hangs. `entityId` omitted means every record of that kind: a
 * send files a message whose id the client does not hold. `companyId` is the
 * company the server stamped: undefined when unknown, null when there is none.
 */
export type FileHome = Readonly<{
  entityType: Attachment["entity_type"];
  entityId?: string;
  companyId?: string | null;
}>;

/** The parent of a stored file, read off the row the server returned. */
export function homeOf(file: Attachment): FileHome {
  return {
    entityType: file.entity_type,
    entityId: file.entity_id,
    companyId: file.company_id,
  };
}

/**
 * What a write did to the files under a parent. An upload adds a file no room
 * shares yet; a move (relink, merge, filing a thread) or a removal can take a
 * shared file out of a deal, and so out of its rooms.
 */
export type FileChange = "uploaded" | "moved" | "removed";

/** The lists a write to the files under `home` leaves stale. */
export function fileListKeys(
  home: FileHome,
  change: FileChange = "uploaded",
): QueryKey[] {
  const own = (prefix: QueryKey, key: (id: string) => QueryKey) =>
    home.entityId === undefined ? prefix : key(home.entityId);
  const rooms = change === "uploaded" ? [] : [ROOM_DOCUMENTS];
  switch (home.entityType) {
    case "contact":
      // A contact upload is stamped with no company: an employee of two has
      // no single account (accountRollUp in attachment.go).
      return [own(CONTACT_FILES, contactFilesKey)];
    case "company":
      return [own(COMPANY_DOCUMENTS, companyDocumentsKey)];
    case "deal":
      return [own(DEAL_FILES, dealFilesKey), ...stampedCompany(home), ...rooms];
    case "activity":
      return [
        own(ACTIVITY_FILES, activityFilesKey),
        CONTACT_FILES,
        DEAL_FILES,
        COMPANY_DOCUMENTS,
        ...rooms,
      ];
    default:
      // No Documents list reads a lead's or a project's own uploads.
      return [];
  }
}

function stampedCompany(home: FileHome): QueryKey[] {
  if (home.companyId === null) return [];
  if (home.companyId === undefined) return [COMPANY_DOCUMENTS];
  return [companyDocumentsKey(home.companyId)];
}

/** What hiding a captured file on one deal changes: that deal's area and the
 *  rooms that say whether each entry still reaches the buyer. */
export function dealHideKeys(dealId: string): QueryKey[] {
  return [dealFilesKey(dealId), ROOM_DOCUMENTS];
}
