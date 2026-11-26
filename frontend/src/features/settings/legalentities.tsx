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
import { useFieldFaults } from "../../design-system/savepage";
import { jurisdictionName } from "../../format/jurisdiction";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import {
  type CreateLegalEntity,
  type LegalEntity,
  useCreateLegalEntity,
  useLegalEntities,
} from "../../lib/legalentities.queries";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { useInstallationJurisdiction } from "./jurisdictionentities";
import {
  ArchiveEntityDialog,
  CountryField,
  EditEntityDialog,
  JurisdictionField,
  TextField,
} from "./legalentityeditor";
import {
  entityLabels,
  entityRefusal,
  missingEntityFields,
} from "./legalentityfields";

// The companies this installation trades as. With one entity the card lists
// the default alone, named after the company above it. An installation that
// never adds a second one sees what it always had.
export function LegalEntitiesCard() {
  const t = useT();
  const { locale } = useLocale();
  const query = useLegalEntities();
  const installation = useInstallationJurisdiction();
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
          columns={entityColumns(
            {
              t,
              locale,
              installationLaw: installation.data?.selected ?? null,
              offered: installation.data?.available.map((unit) => unit.code),
            },
            { ...verbs, onEdit: setEditing, onArchive: setArchiving },
          )}
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

type LawContext = Readonly<{
  t: Translator;
  locale: Locale;
  // The installation's jurisdiction, which the default entity follows.
  installationLaw: string | null;
  // The packs this build offers; undefined until they load.
  offered: readonly string[] | undefined;
}>;

// A further entity with no jurisdiction sends under the installation's, which
// may be the wrong law for it. The list shows an admin the gap.
function jurisdictionCell(
  entity: LegalEntity,
  { t, locale, installationLaw, offered }: LawContext,
) {
  if (!offered) return null;
  const law = entity.is_default ? installationLaw : entity.jurisdiction;
  // A copied address country that no pack covers is flagged, not named.
  if (law && offered.includes(law)) return jurisdictionName(law, locale);
  if (entity.is_default) return t("jurisdiction.entities.none");
  return <Badge tone="warning">{t("legalEntities.jurisdictionMissing")}</Badge>;
}

function entityColumns(
  law: LawContext,
  verbs: RowVerbs,
): DataTableColumn<LegalEntity>[] {
  const { t } = law;
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
    {
      key: "jurisdiction",
      header: t("legalEntities.colJurisdiction"),
      fold: "end",
      render: (entity) => jurisdictionCell(entity, law),
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

const NEW_ENTITY_FIELDS = [
  "display_name",
  "legal_name",
  "country",
  "jurisdiction",
] as const;

const EMPTY_FORM: CreateLegalEntity = {
  display_name: "",
  legal_name: "",
  country: "",
  jurisdiction: "",
};

// The four fields every further entity needs. The rest of its description
// is added in the editor once it exists.
function NewEntityDialog({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const formId = useId();
  const create = useCreateLegalEntity();
  const [form, setForm] = useState<CreateLegalEntity>(EMPTY_FORM);
  const labels = entityLabels(t, NEW_ENTITY_FIELDS);
  const [faults, faultsRoot] = useFieldFaults();
  const refused = entityRefusal(create.error, t, labels);
  const field = (key: (typeof NEW_ENTITY_FIELDS)[number]) => ({
    value: form[key] ?? "",
    error: faults.of(key),
    onChange: (value: string) => {
      setForm((current) => ({ ...current, [key]: value }));
      faults.edited(key);
    },
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (create.isPending) return;
    if (faults.show(missingEntityFields(form, labels, locale))) {
      create.reset();
      return;
    }
    create.mutate(form, {
      onSuccess: onClose,
      onError: (error) => faults.show(entityRefusal(error, t, labels).byField),
    });
  };
  return (
    <RecordFormDialog
      open
      title={t("legalEntities.addTitle")}
      onClose={onClose}
      fieldCount={NEW_ENTITY_FIELDS.length}
      unsaved={NEW_ENTITY_FIELDS.some((key) => (form[key] ?? "").trim() !== "")}
      form={
        <form
          id={formId}
          ref={faultsRoot}
          onSubmit={submit}
          className="form-stack"
          noValidate
        >
          <TextField
            label={t("legalEntities.displayName")}
            required
            maxLength={TEXT_LIMITS.label}
            {...field("display_name")}
          />
          <TextField
            label={t("legalEntities.legalName")}
            required
            maxLength={TEXT_LIMITS.line}
            {...field("legal_name")}
          />
          <CountryField {...field("country")} />
          <JurisdictionField {...field("jurisdiction")} />
          {refused.rest !== null && <ErrorLine>{refused.rest}</ErrorLine>}
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
