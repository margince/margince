// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  Calendar,
  Euro,
  Hash,
  Link2,
  List,
  type LucideIcon,
  ToggleRight,
  Type,
} from "lucide-react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { Switch } from "../../design-system/switch";
import { stable } from "../../format/collate";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { CfObject, CfType } from "../../lib/customfields.logic";
import { useRecordZone } from "../../lib/recordzone";
import { AuditEntryLine } from "../settings/audit";
import { objectLabels, typeLabels } from "./customfields.labels";
import "./customfields.css";

export type CustomField = components["schemas"]["CustomField"];
type AuditLogEntry = components["schemas"]["AuditLogEntry"];

// The id of the optimistic row a create stages before the server commits. A real
// field id is a UUID, so this never collides with one.
export const STAGED_ID = "staged";

/** A per-field switch the settings table offers, by its wire name. */
export type FieldSetting = "show_in_create" | "reportable";

export const SETTING_TOAST = {
  show_in_create: { on: "cf.shownInCreate", off: "cf.hiddenFromCreate" },
  reportable: { on: "cf.addedToReports", off: "cf.removedFromReports" },
} as const;

// Analytics groups deals by their own fields and their company's. It takes only
// a field with a closed set of values, and the server refuses the rest.
const REPORTED_OBJECTS: ReadonlySet<CfObject> = new Set(["deal", "company"]);

function reportableField(object: CfObject, field: CustomField): boolean {
  return (
    REPORTED_OBJECTS.has(object) &&
    (field.type === "picklist" || field.type === "boolean")
  );
}

// Decorative: every use is aria-hidden, and the Type column names the type.
// Partial because a newer server may send a type this build has no icon for.
const TYPE_ICON: Partial<Record<CfType, LucideIcon>> = {
  text: Type,
  number: Hash,
  date: Calendar,
  currency: Euro,
  picklist: List,
  multiselect: List,
  boolean: ToggleRight,
  record: Link2,
};

// One object's fields (AC-custom-fields-1). A retired field stays listed and
// badged, so the history the audit trail keeps stays legible (CUSTOM-FIELDS-AC-13).
export function FieldTable({
  object,
  fields,
  canEdit,
  meUserId,
  onRename,
  onArchive,
  onSetting,
  savingField,
}: Readonly<{
  object: CfObject;
  fields: CustomField[];
  // Both affordances this gates are updates: renaming relabels a live field,
  // and retiring one is a lifecycle transition that keeps the column and its
  // history. Neither is custom_field:delete, which no surface offers.
  canEdit: boolean;
  meUserId?: string;
  onRename: (field: CustomField) => void;
  onArchive: (field: CustomField) => void;
  onSetting: (field: CustomField, setting: FieldSetting, on: boolean) => void;
  // The id of the field whose settings are being written. Both its switches
  // wait, so a second flip cannot send a version the first one moved.
  savingField?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();

  const typeWord = (field: CustomField): string => {
    // A type this build does not know prints its own name.
    const base = typeLabels(t)[field.type] ?? field.type;
    if (field.type === "picklist" || field.type === "multiselect") {
      return `${base} · ${field.options?.length ?? 0}`;
    }
    if (field.type === "currency") {
      return `${base} · ${field.currency ?? ""}`;
    }
    return base;
  };

  const columns: DataTableColumn<CustomField>[] = [
    {
      key: "field",
      header: t("cf.col.field"),
      fold: "title",
      render: (field) => <FieldName field={field} />,
    },
    { key: "type", header: t("cf.col.type"), render: typeWord },
    {
      key: "inCreate",
      header: t("cf.col.inCreate"),
      render: (field) => (
        <SettingCell
          field={field}
          canEdit={canEdit}
          on={field.show_in_create !== false}
          label={t("cf.showInCreateFor", { label: field.label })}
          pending={savingField === field.id}
          onChange={(on) => onSetting(field, "show_in_create", on)}
        />
      ),
    },
    ...(REPORTED_OBJECTS.has(object)
      ? [
          {
            key: "inReports",
            header: t("cf.col.inReports"),
            render: (field: CustomField) =>
              reportableField(object, field) ? (
                <SettingCell
                  field={field}
                  canEdit={canEdit}
                  on={field.reportable}
                  label={t("cf.reportableFor", { label: field.label })}
                  pending={savingField === field.id}
                  onChange={(on) => onSetting(field, "reportable", on)}
                />
              ) : null,
          },
        ]
      : []),
    {
      key: "addedBy",
      header: t("cf.col.addedBy"),
      render: (field) => (
        <CellStack>
          {meUserId === field.created_by ? (
            <span>{t("cf.addedByYou")}</span>
          ) : (
            <EntityRef kind="user" id={field.created_by} />
          )}
          <span className="t-caption">
            {formatDate(field.created_at, locale, recordZone)}
          </span>
        </CellStack>
      ),
    },
  ];

  if (canEdit) {
    columns.push({
      key: "actions",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (field) => (
        <FieldMenu
          field={field}
          onRename={() => onRename(field)}
          onArchive={() => onArchive(field)}
        />
      ),
    });
  }

  return (
    <DataTable
      bleed
      fold
      label={t("cf.listLabel", { object: objectLabels(t)[object] })}
      columns={columns}
      rows={fields}
      rowKey={(field) => field.id}
      rowTestId={(field) => `field-${field.id}`}
    />
  );
}

function FieldName({ field }: Readonly<{ field: CustomField }>) {
  const t = useT();
  const Icon = TYPE_ICON[field.type] ?? Type;
  return (
    <span
      className={
        field.id === STAGED_ID ? "cf-fieldcell cf-cell-staged" : "cf-fieldcell"
      }
    >
      <span className="cf-fieldicon" aria-hidden>
        <Icon />
      </span>
      <span>{field.label}</span>
      {field.status === "retired" && <Badge>{t("cf.retired")}</Badge>}
    </span>
  );
}

// A switch where the reader may flip it, and the plain answer where they may
// not. A staged row has no id yet, and a retired field's settings are frozen.
// The create-form setting reads `!== false`, so a server older than the
// setting reads as on, which it was.
function SettingCell({
  field,
  canEdit,
  on,
  label,
  pending,
  onChange,
}: Readonly<{
  field: CustomField;
  canEdit: boolean;
  on: boolean;
  label: string;
  pending: boolean;
  onChange: (on: boolean) => void;
}>) {
  const t = useT();
  if (!canEdit || field.id === STAGED_ID || field.status === "retired") {
    return <span>{on ? t("field.yes") : t("field.no")}</span>;
  }
  return (
    <Switch
      label={label}
      labelHidden
      checked={on}
      pending={pending}
      onChange={onChange}
    />
  );
}

// A staged row has no id the server would honour yet, and a retired field has
// no verb left: the API offers no restore.
function FieldMenu({
  field,
  onRename,
  onArchive,
}: Readonly<{
  field: CustomField;
  onRename: () => void;
  onArchive: () => void;
}>) {
  const t = useT();
  if (field.id === STAGED_ID) {
    return <span className="cf-cell-staged">{t("cf.writing")}</span>;
  }
  if (field.status === "retired") {
    return null;
  }
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: field.label })}>
        <Button aria-haspopup="dialog" onClick={onRename}>
          {t("cf.edit")}
        </Button>
        <Button aria-haspopup="dialog" onClick={onArchive}>
          {t("cf.archive")}
        </Button>
      </OverflowMenu>
    </span>
  );
}

// The audit rows this screen's changes emit, newest first (AC-custom-fields-6/7).
// Only what AuditLogEntry carries is drawn, never an invented display name.
export function AuditRail({
  entries,
  state,
  meUserId,
  onRetry,
}: Readonly<{
  entries: AuditLogEntry[];
  state: SectionState;
  meUserId?: string;
  onRetry: () => void;
}>) {
  const t = useT();
  const recentFirst = [...entries].sort((a, b) =>
    stable(b.occurred_at, a.occurred_at),
  );

  return (
    <SurfaceState
      loadingLabel={t("cf.audit.title")}
      state={state}
      emptyLabel={t("cf.audit.empty")}
      detail={{ onRetry }}
    >
      <ul className="cf-audit">
        {recentFirst.map((entry) => (
          <li key={entry.id}>
            <AuditEntryLine entry={entry} meUserId={meUserId} />
          </li>
        ))}
      </ul>
    </SurfaceState>
  );
}

// Withheld is checked first: a disabled query stays pending, so a reader without
// the grant would otherwise see a skeleton forever.
export function auditState(
  readsTrail: boolean,
  isPending: boolean,
  isError: boolean,
  count: number,
): SectionState {
  if (!readsTrail) {
    return "withheld";
  }
  if (isError) {
    return "failed";
  }
  if (isPending) {
    return "loading";
  }
  return count === 0 ? "empty" : "ready";
}
