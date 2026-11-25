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
import { entryName } from "../settings/audit";

type AuditLogEntry = components["schemas"]["AuditLogEntry"];
type CustomField = components["schemas"]["CustomField"];
type Image = NonNullable<AuditLogEntry["after"]>;
type Setting = keyof CustomField;

const moved = (image: Image, setting: Setting) => setting in image;

function switched(
  image: Image,
  setting: Setting,
  keys: Readonly<{ on: MessageKey; off: MessageKey }>,
): MessageKey | undefined {
  const value: unknown = image[setting];
  if (typeof value !== "boolean") return undefined;
  return value ? keys.on : keys.off;
}

// An update's images hold only the settings it moved, under the field's own
// contract names. Each rule reads one of them from the image after the change.
const UPDATE_RULES: readonly ((after: Image) => MessageKey | undefined)[] = [
  (after) => (moved(after, "label") ? "cf.audit.renamed" : undefined),
  (after) => (moved(after, "options") ? "cf.audit.choices" : undefined),
  // A change of choices rewrites their translations too, and is not one.
  (after) =>
    moved(after, "label_translations") ||
    (moved(after, "option_translations") && !moved(after, "options"))
      ? "cf.audit.translations"
      : undefined,
  (after) =>
    switched(after, "show_in_create", {
      on: "cf.audit.shownInCreate",
      off: "cf.audit.hiddenFromCreate",
    }),
  (after) =>
    switched(after, "reportable", {
      on: "cf.audit.addedToReports",
      off: "cf.audit.removedFromReports",
    }),
  (after) => (moved(after, "activity_kinds") ? "cf.audit.kinds" : undefined),
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
  return UPDATE_RULES.flatMap((rule) => rule(after) ?? []);
}

/**
 * The field an entry changed, as "Contact · Budget". It comes from the catalog
 * row the reader holds, else from the object and label the entry carries.
 */
export function fieldTarget(
  entry: AuditLogEntry,
  field: CustomField | undefined,
  t: Translator,
  locale: string,
): string | undefined {
  const written: unknown = entry.after?.object;
  const object: CfObject | undefined =
    field?.object ?? CF_OBJECTS.find((one) => one === written);
  const name = field ? customFieldLabel(field, locale) : entryName(entry);
  if (object === undefined) return name;
  const objectName = t(`cf.obj.${object}`);
  return name ? `${objectName} · ${name}` : objectName;
}
