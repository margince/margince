// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ExtensionDescriptor,
  type ExtensionEntryDescriptor,
  extensions,
} from "@composition/extensions";
import { type EntityKind, isEntityKind } from "./entity";
import { EXTENSION_SCREEN } from "./extensions";
import { type Route, useRoute } from "./router";
import { NO_URL_PARAMS, type UrlParams, useUrlParams } from "./urlstate";

// A unit's entries: the ways into its own screen that core screens offer. The
// unit declares the slot, the copy key, the glyph and the view. The screen that
// owns the slot decides where the entry sits and how it looks.

/**
 * Every slot a core screen renders. Mirrors extension.EntrySlots() in
 * backend/pkg/extension/entry.go; backend/gates/frontendentryslots_test.go
 * fails when the two lists differ.
 */
export const EXTENSION_ENTRY_SLOTS = [
  "log_activity",
  "record_header.contact",
  "record_header.company",
  "record_header.deal",
  "record_mark.contact",
  "record_mark.company",
  "record_mark.deal",
] as const;

export type ExtensionEntrySlot = (typeof EXTENSION_ENTRY_SLOTS)[number];

/** One entry with the unit that declared it, which is where it leads. */
export type UnitEntry = ExtensionEntryDescriptor & { readonly unit: string };

/** The record an entry was pressed on, handed to the unit for prefill. */
export type EntryRecord = { readonly type: EntityKind; readonly id: string };

// The query keys a unit screen reads the record from. Spelled once, so the
// writer below and the reader in useEntryContext cannot drift.
export const ENTRY_RECORD_TYPE_PARAM = "record_type";
export const ENTRY_RECORD_ID_PARAM = "record_id";

/** Every composed unit's entries for `slot`, in registry order. */
export function entriesForSlot(
  slot: ExtensionEntrySlot,
  registry: readonly ExtensionDescriptor[] = extensions,
): readonly UnitEntry[] {
  return registry.flatMap((unit) =>
    unit.entries
      .filter((entry) => entry.slot === slot)
      .map((entry) => ({ ...entry, unit: unit.name })),
  );
}

/** The unit screen an entry opens: `#/ext/<unit>`, or one view under it. */
export function entryRoute(entry: UnitEntry): Route {
  return {
    screen: EXTENSION_SCREEN,
    id: entry.unit,
    id2: entry.target || undefined,
  };
}

/** The query an entry hands the unit: the record it was pressed on, if any. */
export function entryParams(record: EntryRecord | undefined): UrlParams {
  if (!record) {
    return NO_URL_PARAMS;
  }
  return new Map([
    [ENTRY_RECORD_TYPE_PARAM, record.type],
    [ENTRY_RECORD_ID_PARAM, record.id],
  ]);
}

/**
 * The record a unit screen was opened on, or null. A hand-typed address can
 * name any kind, so one this app does not route to reads as no record.
 */
export function entryRecordFromParams(params: UrlParams): EntryRecord | null {
  const type = params.get(ENTRY_RECORD_TYPE_PARAM);
  const id = params.get(ENTRY_RECORD_ID_PARAM);
  if (!type || !id || !isEntityKind(type)) {
    return null;
  }
  return { type, id };
}

/**
 * What a unit screen was opened with: the view segment under `#/ext/<unit>`
 * ("" for the screen itself) and the record an entry was pressed on.
 */
export function useEntryContext(): {
  readonly view: string;
  readonly record: EntryRecord | null;
} {
  const route = useRoute();
  const [params] = useUrlParams();
  return {
    view: route.screen === EXTENSION_SCREEN ? (route.id2 ?? "") : "",
    record: entryRecordFromParams(params),
  };
}
