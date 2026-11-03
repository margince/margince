// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useId, useState } from "react";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { type Translator, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf } from "../../lib/problem";
import {
  type CreateLegalEntity,
  type LegalEntity,
  useCreateLegalEntity,
  useLegalEntities,
} from "./legalentities.queries";
import {
  ArchiveEntityDialog,
  CountryField,
  EditEntityDialog,
  TextField,
} from "./legalentityeditor";

// The companies this installation trades as. With one entity the card lists
// the default alone, named after the company above it. An installation that
// never adds a second one sees what it always had.
export function LegalEntitiesCard() {
  const t = useT();
  const query = useLegalEntities();
  const canCreate = useCanWrite("legal_entity", "create");
  const verbs = {
    canEdit: useCanWrite("legal_entity", "update"),
    canArchive: useCanWrite("legal_entity", "delete"),
  };
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<LegalEntity | null>(null);
  const [archiving, setArchiving] = useState<LegalEntity | null>(null);
  const entities = query.data ?? [];
  return (
    <Panel
      title={t("legalEntities.title")}
      titleAction={
        canCreate && (
          <Button onClick={() => setOpen(true)}>
            {t("legalEntities.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("legalEntities.sub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && entities.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("legalEntities.title")}
          columns={entityColumns(t, {
            ...verbs,
            onEdit: setEditing,
            onArchive: setArchiving,
          })}
          rows={entities}
          rowKey={(entity) => entity.id}
          rowTestId={(entity) => `legal-entity-${entity.id}`}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("legalEntities.loading")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      {open && <NewEntityDialog onClose={() => setOpen(false)} />}
      {editing && (
        <EditEntityDialog entity={editing} onClose={() => setEditing(null)} />
      )}
      <ArchiveEntityDialog
        entity={archiving}
        onClose={() => setArchiving(null)}
      />
    </Panel>
  );
}

type RowVerbs = Readonly<{
  canEdit: boolean;
  canArchive: boolean;
  onEdit: (entity: LegalEntity) => void;
  onArchive: (entity: LegalEntity) => void;
}>;

function entityColumns(
  t: Translator,
  verbs: RowVerbs,
): DataTableColumn<LegalEntity>[] {
  const columns: DataTableColumn<LegalEntity>[] = [
    {
      key: "name",
      header: t("legalEntities.colName"),
      grow: true,
      render: (entity) => (
        <span className="t-name">
          {nameOf(entity, t)}{" "}
          {entity.is_default && (
            <Badge>{t("legalEntities.defaultBadge")}</Badge>
          )}
        </span>
      ),
    },
    {
      key: "legal",
      header: t("legalEntities.colLegalName"),
      render: (entity) => entity.legal_name ?? "",
    },
    {
      key: "country",
      header: t("legalEntities.colCountry"),
      fold: "end",
      render: (entity) => entity.country?.toUpperCase() ?? "",
    },
  ];
  if (!verbs.canEdit && !verbs.canArchive) return columns;
  return [
    ...columns,
    {
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      fold: "end",
      render: (entity) => (
        <span className="cell-actions">
          <OverflowMenu
            label={t("table.rowActions", { name: nameOf(entity, t) })}
          >
            {verbs.canEdit && (
              <Button onClick={() => verbs.onEdit(entity)}>
                {t("legalEntities.edit")}
              </Button>
            )}
            {/* The default is the entity every unassigned colleague works
                for, so it is never offered for archiving. */}
            {verbs.canArchive && !entity.is_default && (
              <Button onClick={() => verbs.onArchive(entity)}>
                {t("legalEntities.archive")}
              </Button>
            )}
          </OverflowMenu>
        </span>
      ),
    },
  ];
}

function nameOf(entity: LegalEntity, t: Translator): string {
  return entity.display_name || t("legalEntities.defaultUnnamed");
}

const EMPTY_FORM: CreateLegalEntity = {
  display_name: "",
  legal_name: "",
  country: "",
};

// The three fields every further entity needs. The rest of its description
// is added in the editor once it exists.
function NewEntityDialog({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const formId = useId();
  const create = useCreateLegalEntity();
  const [form, setForm] = useState<CreateLegalEntity>(EMPTY_FORM);
  const set = (key: keyof CreateLegalEntity) => (value: string) =>
    setForm((current) => ({ ...current, [key]: value }));
  const submit = (event: FormEvent) => {
    event.preventDefault();
    create.mutate(form, { onSuccess: onClose });
  };
  return (
    <RecordFormDialog
      open
      title={t("legalEntities.addTitle")}
      onClose={onClose}
      fieldCount={3}
      unsaved={[form.display_name, form.legal_name, form.country].some(
        (value) => (value ?? "").trim() !== "",
      )}
      form={
        <form id={formId} onSubmit={submit} className="form-stack">
          <TextField
            label={t("legalEntities.displayName")}
            required
            value={form.display_name}
            onChange={set("display_name")}
          />
          <TextField
            label={t("legalEntities.legalName")}
            required
            value={form.legal_name}
            onChange={set("legal_name")}
          />
          <CountryField
            required
            value={form.country}
            onChange={set("country")}
          />
          <ErrorLine
            error={create.isError && problemMessageOf(create.error, t)}
          />
        </form>
      }
      actions={
        <>
          <Button disabled={create.isPending} onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            pending={create.isPending}
          >
            {t("legalEntities.addConfirm")}
          </Button>
        </>
      }
    />
  );
}
