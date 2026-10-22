// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useId, useState } from "react";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  TextInput,
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

// The companies this installation trades as. With one entity the card lists
// the default alone, named after the company above it. An installation that
// never adds a second one sees what it always had.
export function LegalEntitiesCard() {
  const t = useT();
  const query = useLegalEntities();
  const canCreate = useCanWrite("legal_entity", "create");
  const [open, setOpen] = useState(false);
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
          columns={entityColumns(t)}
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
    </Panel>
  );
}

function entityColumns(t: Translator): DataTableColumn<LegalEntity>[] {
  return [
    {
      key: "name",
      header: t("legalEntities.colName"),
      grow: true,
      render: (entity) => (
        <span className="t-name">
          {entity.display_name || t("legalEntities.defaultUnnamed")}{" "}
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
}

const EMPTY_FORM: CreateLegalEntity = {
  display_name: "",
  legal_name: "",
  country: "",
};

// The three fields every further entity needs. Its address, bank details and
// imprint are described through the API until the editor that shows them lands.
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
            value={form.display_name}
            onChange={set("display_name")}
          />
          <TextField
            label={t("legalEntities.legalName")}
            value={form.legal_name}
            onChange={set("legal_name")}
          />
          <TextField
            label={t("legalEntities.country")}
            hint={t("legalEntities.countryHint")}
            value={form.country}
            maxLength={2}
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

function TextField({
  label,
  hint,
  value,
  maxLength,
  onChange,
}: Readonly<{
  label: string;
  hint?: string;
  value: string;
  maxLength?: number;
  onChange: (value: string) => void;
}>) {
  return (
    <Field label={label} hint={hint} required>
      {(control) => (
        <TextInput
          {...control}
          required
          maxLength={maxLength}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}
