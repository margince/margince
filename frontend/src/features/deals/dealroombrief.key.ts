// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The room brief's query key, in a module of its own. A write that changes
// what the brief reads refreshes it from here, without importing the brief.
export const ROOM_BRIEF_PREFIX = ["deal-room-brief"] as const;

export function roomBriefKey(roomId: string) {
  return [...ROOM_BRIEF_PREFIX, roomId] as const;
}
