// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ExtensionDescriptor } from "@composition/extensions";
import {
  Camera,
  CheckSquare,
  ClipboardList,
  FileText,
  type LucideIcon,
  MapPin,
  MessageSquare,
  Plug,
} from "lucide-react";
import { ActionRow } from "../design-system/actionrow";
import { Badge, Button } from "../design-system/atoms";
import { translate, useLocale } from "../i18n";
import { grants } from "../lib/capability";
import type { EntityKind } from "../lib/entity";
import {
  type EntryRecord,
  type ExtensionEntrySlot,
  entriesForSlot,
  entryParams,
  entryRoute,
  type UnitEntry,
} from "../lib/extensionentries";
import { isExtensionRbacObject } from "../lib/extensions";
import { navigate } from "../lib/router";
import { useMe } from "../lib/session";

/**
 * The glyphs an entry may name. Mirrors extension.EntryIcons() in
 * backend/pkg/extension/entry.go, under backend/gates/frontendentryslots_test.go.
 */
export const ENTRY_ICONS: Readonly<Record<string, LucideIcon>> = {
  "clipboard-list": ClipboardList,
  "file-text": FileText,
  "map-pin": MapPin,
  camera: Camera,
  "check-square": CheckSquare,
  "message-square": MessageSquare,
  plug: Plug,
};

// A slot with this prefix draws each entry as a note, not a button.
const MARK_SLOT_PREFIX = "record_mark.";

/**
 * The composed units' entries for one slot, as buttons that open the unit's
 * screen on `record`. An entry shows only to a reader holding read on the
 * object it names, and a slot with nothing to show draws nothing.
 *
 * `inRow` wraps the buttons in an `ActionRow`, for a slot inside a form; a
 * record header lays them into its own verb row.
 */
export function ExtensionEntries({
  slot,
  record,
  inRow = false,
  registry,
}: Readonly<{
  slot: ExtensionEntrySlot;
  record?: EntryRecord;
  inRow?: boolean;
  registry?: readonly ExtensionDescriptor[];
}>) {
  const { locale } = useLocale();
  const me = useMe();
  const shown = entriesForSlot(slot, registry).filter(
    (entry) =>
      isExtensionRbacObject(entry.object) &&
      grants(me.data, entry.object, "read"),
  );
  if (shown.length === 0) {
    return null;
  }
  if (slot.startsWith(MARK_SLOT_PREFIX)) {
    return shown.map((entry) => (
      <EntryMark
        key={`${entry.unit}/${entry.target}`}
        entry={entry}
        label={translate(locale, entry.label)}
      />
    ));
  }
  const buttons = shown.map((entry) => (
    <EntryButton
      key={`${entry.unit}/${entry.target}`}
      entry={entry}
      label={translate(locale, entry.label)}
      record={record}
    />
  ));
  return inRow ? <ActionRow>{buttons}</ActionRow> : buttons;
}

/**
 * The Log activity form's slot: a unit's own way to log, beside the core
 * kinds. A form opened as Add task files a task, which no unit screen does.
 */
export function LogActivityEntries({
  onlyTasks,
  entityType,
  entityId,
}: Readonly<{
  onlyTasks: boolean;
  entityType?: EntityKind;
  entityId?: string;
}>) {
  if (onlyTasks) {
    return null;
  }
  return (
    <ExtensionEntries
      slot="log_activity"
      inRow
      record={
        entityType && entityId ? { type: entityType, id: entityId } : undefined
      }
    />
  );
}

function EntryMark({
  entry,
  label,
}: Readonly<{ entry: UnitEntry; label: string }>) {
  const icon = Object.hasOwn(ENTRY_ICONS, entry.icon)
    ? ENTRY_ICONS[entry.icon]
    : undefined;
  return <Badge icon={icon}>{label}</Badge>;
}

function EntryButton({
  entry,
  label,
  record,
}: Readonly<{ entry: UnitEntry; label: string; record?: EntryRecord }>) {
  // Object.hasOwn: the map is an object literal, so a bare index would answer
  // "constructor" from the prototype.
  const Icon = Object.hasOwn(ENTRY_ICONS, entry.icon)
    ? ENTRY_ICONS[entry.icon]
    : undefined;
  return (
    <Button
      data-testid={`ext-entry-${entry.unit}`}
      onClick={() => navigate(entryRoute(entry), entryParams(record))}
    >
      {Icon && <Icon aria-hidden="true" />} {label}
    </Button>
  );
}
