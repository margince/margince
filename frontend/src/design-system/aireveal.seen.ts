// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readStoredJson, STORAGE_KEYS, writeStored } from "../lib/storage";

// Enough for weeks of drafts, briefs and answers; the oldest fall off first.
const KEEP = 400;

// Without Web Storage the record lives as long as the tab does.
const seenInTab = new Set<string>();

function readSeen(): string[] {
  return (
    readStoredJson(STORAGE_KEYS.aiRevealSeen, (value) =>
      Array.isArray(value)
        ? value.filter((print): print is string => typeof print === "string")
        : null,
    ) ?? []
  );
}

/**
 * A short, stable stand-in for the text (FNV-1a, 32 bits), so the record holds
 * no words a model wrote and stays small whatever their length.
 */
export function fingerprint(text: string): string {
  let hash = 0x811c9dc5;
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(36);
}

export function revealedBefore(print: string): boolean {
  return seenInTab.has(print) || readSeen().includes(print);
}

export function markRevealed(print: string): void {
  seenInTab.add(print);
  const kept = readSeen().filter((stored) => stored !== print);
  kept.push(print);
  writeStored(STORAGE_KEYS.aiRevealSeen, JSON.stringify(kept.slice(-KEEP)));
}
