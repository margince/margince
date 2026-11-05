// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The room documents' query key, in a module of its own. A change to the deal's
// files refreshes every room's list from here, without importing the list.
export const ROOM_DOCUMENTS_PREFIX = ["deal-room-documents"] as const;

export function roomDocumentsKey(roomId: string) {
  return [...ROOM_DOCUMENTS_PREFIX, roomId] as const;
}
