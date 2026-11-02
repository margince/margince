// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { foldForSearch } from "../format/collate";
import { type EntityKind, isEntityKind } from "../lib/entity";
import { readStoredJson, STORAGE_KEYS, writeStored } from "../lib/storage";
import { isUuid } from "../lib/uuid";

export type RecentEntry =
  | { kind: "query"; text: string }
  | { kind: "record"; entity_type: EntityKind; id: string; label: string };
export type RecentDocument = {
  version: 1;
  seat: string;
  entries: RecentEntry[];
};
const MAX_ENTRIES = 8;
const MAX_TEXT = 4096;
function text(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.trim().length > 0 &&
    value.length <= MAX_TEXT
  );
}

function parseEntry(value: unknown): RecentEntry | null {
  if (typeof value !== "object" || value === null || !("kind" in value))
    return null;
  if (value.kind === "query" && "text" in value && text(value.text))
    return { kind: "query", text: value.text };
  if (
    value.kind === "record" &&
    "entity_type" in value &&
    typeof value.entity_type === "string" &&
    isEntityKind(value.entity_type) &&
    "id" in value &&
    text(value.id) &&
    isUuid(value.id) &&
    "label" in value &&
    text(value.label)
  )
    return {
      kind: "record",
      entity_type: value.entity_type,
      id: value.id.toLowerCase(),
      label: value.label,
    };
  return null;
}
export function recentIdentity(entry: RecentEntry): string {
  return entry.kind === "query"
    ? `query:${foldForSearch(entry.text.trim().replace(/\s+/g, " "))}`
    : `record:${entry.entity_type}:${entry.id.toLowerCase()}`;
}
export function boundedRecents(entries: readonly RecentEntry[]): RecentEntry[] {
  const seen = new Set<string>();
  return entries
    .filter((entry) => {
      const key = recentIdentity(entry);
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .slice(0, MAX_ENTRIES);
}
export function readRecents(seat: string): RecentDocument {
  const parsed = readStoredJson(
    STORAGE_KEYS.paletteRecents,
    (value: unknown): RecentDocument | null => {
      if (
        typeof value !== "object" ||
        value === null ||
        !("version" in value) ||
        value.version !== 1 ||
        !("seat" in value) ||
        value.seat !== seat ||
        !("entries" in value) ||
        !Array.isArray(value.entries)
      )
        return null;
      const entries: RecentEntry[] = [];
      const seen = new Set<string>();
      for (const candidate of value.entries) {
        const entry = parseEntry(candidate);
        if (!entry || seen.has(recentIdentity(entry))) continue;
        seen.add(recentIdentity(entry));
        entries.push(entry);
        if (entries.length === MAX_ENTRIES) break;
      }
      return { version: 1, seat, entries: boundedRecents(entries) };
    },
  );
  return parsed ?? { version: 1, seat, entries: [] };
}
export function writeRecents(seat: string, entries: readonly RecentEntry[]) {
  writeStored(
    STORAGE_KEYS.paletteRecents,
    JSON.stringify({
      version: 1,
      seat,
      entries: boundedRecents(
        entries.flatMap((entry) => {
          const parsed = parseEntry(entry);
          return parsed ? [parsed] : [];
        }),
      ),
    }),
  );
}
