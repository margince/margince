// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  Calendar,
  Euro,
  Hash,
  List,
  type LucideIcon,
  ToggleRight,
  Type,
} from "lucide-react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, OverflowMenu } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { KeyedName } from "../design-system/keyedname";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { stable } from "../format/collate";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import { AuditEntryLine } from "./audit";
import { objectLabels } from "./customfields.labels";
import type { CfObject, CfType } from "./customfields.logic";
import { EntityRef } from "./entityref";
import "./customfields.css";

export type CustomField = components["schemas"]["CustomField"];
type AuditLogEntry = components["schemas"]["AuditLogEntry"];

// The id of the optimistic row a create stages before the server commits. A real
// field id is a UUID, so this never collides with one.
export const STAGED_ID = "staged";

// Decorative: every use is aria-hidden, and the Type column names the type.
const TYPE_ICON: Record<CfType, LucideIcon> = {
  text: Type,
  number: Hash,
  date: Calendar,
  currency: Euro,
  picklist: List,
  multiselect: List,
  boolean: ToggleRight,
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
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();

  const typeWord = (field: CustomField): string => {
    const base = t(`cf.type.${field.type}`);
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
  const Icon = TYPE_ICON[field.type];
  return (
    <span
      className={
        field.id === STAGED_ID ? "cf-fieldcell cf-cell-staged" : "cf-fieldcell"
      }
    >
      <span className="cf-fieldicon" aria-hidden>
        <Icon />
      </span>
      <KeyedName
        name={field.label}
        code={`${field.object}.${field.column_name}`}
      />
      {field.status === "retired" && <Badge>{t("cf.retired")}</Badge>}
    </span>
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
      <OverflowMenu label={t("cf.rowActions", { label: field.label })}>
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
