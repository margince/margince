// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  CF_OBJECTS,
  type CfObject,
  customFieldLabel,
} from "../../lib/customfields.logic";

type AuditLogEntry = components["schemas"]["AuditLogEntry"];
type CustomField = components["schemas"]["CustomField"];
type Image = NonNullable<AuditLogEntry["after"]>;
type Setting = keyof CustomField;

// A write records every setting its request carried, changed or not, so a
// setting moved only where its value before and after the write differ.
const moved = (before: Image, after: Image, setting: Setting) =>
  setting in after &&
  JSON.stringify(before[setting]) !== JSON.stringify(after[setting]);

function switched(
  before: Image,
  after: Image,
  setting: Setting,
  keys: Readonly<{ on: MessageKey; off: MessageKey }>,
): MessageKey | undefined {
  if (!moved(before, after, setting)) return undefined;
  const value: unknown = after[setting];
  if (typeof value !== "boolean") return undefined;
  return value ? keys.on : keys.off;
}

// An update's images hold the settings its request carried, under the
// field's own contract names. Each rule reads one of them.
const UPDATE_RULES: readonly ((
  before: Image,
  after: Image,
) => MessageKey | undefined)[] = [
  (before, after) =>
    moved(before, after, "label") ? "cf.audit.renamed" : undefined,
  (before, after) =>
    moved(before, after, "options") ? "cf.audit.choices" : undefined,
  // A change of choices rewrites their translations too, and is not one.
  (before, after) =>
    moved(before, after, "label_translations") ||
    (moved(before, after, "option_translations") &&
      !moved(before, after, "options"))
      ? "cf.audit.translations"
      : undefined,
  (before, after) =>
    switched(before, after, "show_in_create", {
      on: "cf.audit.shownInCreate",
      off: "cf.audit.hiddenFromCreate",
    }),
  (before, after) =>
    switched(before, after, "reportable", {
      on: "cf.audit.addedToReports",
      off: "cf.audit.removedFromReports",
    }),
  (before, after) =>
    moved(before, after, "activity_kinds") ? "cf.audit.kinds" : undefined,
];

/**
 * What one audit entry did to its custom field, as catalog keys. Empty for a
 * change this build cannot name, which leaves the entry's own verb to say it.
 */
export function fieldChangeKeys(entry: AuditLogEntry): readonly MessageKey[] {
  if (entry.action === "create") return ["audit.action.create"];
  const after = entry.after ?? {};
  // An archive also takes the field out of reports, as its own consequence.
  const status: Setting = "status";
  if (after[status] === "retired") return ["audit.action.archive"];
  const before = entry.before ?? {};
  return UPDATE_RULES.flatMap((rule) => rule(before, after) ?? []);
}

/**
 * The field an entry changed, as "Contact · Budget". It comes from the catalog
 * row the reader holds, else from the object and the name the entry recorded.
 */
export function fieldTarget(
  entry: AuditLogEntry,
  field: CustomField | undefined,
  recorded: string | undefined,
  t: Translator,
  locale: string,
): string | undefined {
  const written: unknown = entry.after?.object;
  const object: CfObject | undefined =
    field?.object ?? CF_OBJECTS.find((one) => one === written);
  const name = field ? customFieldLabel(field, locale) : recorded;
  if (object === undefined) return name;
  const objectName = t(`cf.obj.${object}`);
  return name ? `${objectName} · ${name}` : objectName;
}
