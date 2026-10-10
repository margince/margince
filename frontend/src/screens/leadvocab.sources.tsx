// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { isOption } from "../app/options";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { KeyedName } from "../design-system/keyedname";
import { NameDialog } from "../design-system/namedialog";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { Select } from "../design-system/select";
import { Switch } from "../design-system/switch";
import { formatNumber } from "../format/format";
import {
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { Locale } from "../i18n/locale";
import { problemMessageOf, QueryStates, unwrap } from "./common";
import { LEAD_LIST_KEY } from "./leadkeys";
import {
  type DiscoveredLeadSource,
  LEAD_SOURCES_KEY,
  type LeadSource,
  type LeadSourceIntent,
  sourceKeyLabel,
  useLeadSources,
} from "./leadsources";
import {
  LeadCount,
  rowsOf,
  VocabNotices,
  VocabRowMenu,
} from "./leadvocab.rows";

const INTENTS = ["high", "neutral", "low"] as const;

const intentLabel: Record<LeadSourceIntent, MessageKey> = {
  high: "leadSources.intent.high",
  neutral: "leadSources.intent.neutral",
  low: "leadSources.intent.low",
};

function intentOptions(t: Translator) {
  return INTENTS.map((value) => ({ value, label: t(intentLabel[value]) }));
}

type SourcePatch = {
  id: string;
  label?: string;
  intent?: LeadSourceIntent;
  active?: boolean;
};
type NewSource = { label: string; key?: string; intent: LeadSourceIntent };

async function createSource(body: NewSource) {
  return unwrap(await api.POST("/lead-sources", { body }));
}

async function patchSource({ id, ...body }: SourcePatch) {
  return unwrap(
    await api.PATCH("/lead-sources/{id}", { params: { path: { id } }, body }),
  );
}

// One mutation per write, so a refusal speaks where it was asked. The dialogs
// own create, rename and remove; the card owns adopt and the row controls.
function useSourceMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: LEAD_SOURCES_KEY });
    // The leads list and its create form render labels off this list.
    void queryClient.invalidateQueries({ queryKey: LEAD_LIST_KEY });
  };
  const create = useMutation({
    mutationFn: createSource,
    onSuccess: invalidate,
  });
  const adopt = useMutation({
    mutationFn: createSource,
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: patchSource,
    onSuccess: invalidate,
  });
  const rename = useMutation({
    mutationFn: patchSource,
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.DELETE("/lead-sources/{id}", { params: { path: { id } } }),
      );
    },
    onSuccess: invalidate,
  });
  return { create, adopt, update, rename, remove };
}

// Mounted only while open, so a half-typed source never waits under an intent
// nobody re-chose.
function AddSourceDialog({
  create,
  onClose,
}: Readonly<{
  create: ReturnType<typeof useSourceMutations>["create"];
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  const [label, setLabel] = useState("");
  const [intent, setIntent] = useState<LeadSourceIntent>("neutral");
  const ready = label.trim() !== "";
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" className="t-h2 modal-title" id={titleId}>
        {t("leadSources.newLabel")}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(e) => {
          e.preventDefault();
          if (!ready || create.isPending) return;
          create.mutate(
            { label: label.trim(), intent },
            { onSuccess: onClose },
          );
        }}
      >
        {create.isError && (
          <Callout
            kind="outcome"
            tone="danger"
            title={t("leadSources.notAdded")}
          >
            {problemMessageOf(create.error, t)}
          </Callout>
        )}
        <Field label={t("leadSources.labelField")}>
          {(control) => (
            <TextInput
              {...control}
              data-testid="lead-source-new-label"
              placeholder={t("leadSources.newPlaceholder")}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
          )}
        </Field>
        <Field
          label={t("leadSources.intent")}
          hint={t("leadSources.intentHint")}
        >
          {(control) => (
            <Select
              {...control}
              value={intent}
              onChange={(value) => {
                if (isOption(value, INTENTS)) setIntent(value);
              }}
              options={intentOptions(t)}
            />
          )}
        </Field>
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("deals.cancel")}
        </Button>
        {/* Not one `disabled`: an empty form and a write on its way read differently. */}
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={!create.isPending && !ready}
          pending={create.isPending}
        >
          {t("leadSources.add")}
        </Button>
      </div>
    </Modal>
  );
}

function sourceColumns({
  t,
  plural,
  locale,
  grants,
  onUpdate,
  onRename,
  onRemove,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  grants: Readonly<{ canEdit: boolean; canRemove: boolean }>;
  onUpdate: (patch: SourcePatch) => void;
  onRename: (source: LeadSource) => void;
  onRemove: (source: LeadSource) => void;
}>): DataTableColumn<LeadSource>[] {
  const { canEdit, canRemove } = grants;
  const refusal = (source: LeadSource) => {
    const count = source.lead_count ?? 0;
    if (source.system === true) return t("leadSources.builtInKept");
    if (count > 0) {
      return plural("leadSources.inUse", count, {
        count: formatNumber(count, locale),
      });
    }
    return undefined;
  };
  return [
    {
      key: "name",
      header: t("leadSources.colSource"),
      grow: true,
      render: (source) => (
        <span className="lead-vocab-name">
          <KeyedName name={source.label} code={source.key} />
          {/* Only a seat that may remove reads it: built-in is why Remove is refused. */}
          {canRemove && source.system === true && (
            <Badge>{t("leadSources.builtIn")}</Badge>
          )}
        </span>
      ),
    },
    {
      key: "leads",
      header: t("leadSources.colLeads"),
      align: "end",
      render: (source) => <LeadCount count={source.lead_count ?? 0} />,
    },
    {
      key: "intent",
      header: t("leadSources.intent"),
      render: (source) => (
        <span className="lead-vocab-intent">
          <Select
            aria-label={t("leadSources.intentFor", { label: source.label })}
            value={source.intent}
            disabled={!canEdit}
            onChange={(value) => {
              if (isOption(value, INTENTS) && value !== source.intent) {
                onUpdate({ id: source.id, intent: value });
              }
            }}
            options={intentOptions(t)}
          />
        </span>
      ),
    },
    {
      key: "active",
      header: t("leadSources.colActive"),
      render: (source) => (
        <Switch
          label={t("leadSources.activeFor", { label: source.label })}
          labelHidden
          checked={source.active}
          disabled={!canEdit}
          onChange={(next) => onUpdate({ id: source.id, active: next })}
        />
      ),
    },
    {
      key: "verbs",
      header: t("leadSources.colActions"),
      headerHidden: true,
      fold: "end",
      render: (source) => (
        <VocabRowMenu
          label={source.label}
          canEdit={canEdit}
          canRemove={canRemove}
          refusal={refusal(source)}
          onRename={() => onRename(source)}
          onRemove={() => onRemove(source)}
        />
      ),
    },
  ];
}

// Values a lead carries that the list does not name yet, one verb each.
function DiscoveredSources({
  discovered,
  administered,
  canCreate,
  onAdopt,
}: Readonly<{
  discovered: readonly DiscoveredLeadSource[];
  administered: readonly LeadSource[];
  canCreate: boolean;
  onAdopt: (found: DiscoveredLeadSource, label: string) => void;
}>) {
  const t = useT();
  const columns: DataTableColumn<DiscoveredLeadSource>[] = [
    {
      key: "name",
      header: t("leadSources.colSource"),
      grow: true,
      render: (found) => (
        <KeyedName
          name={sourceKeyLabel(found.key, administered, t)}
          code={found.key}
        />
      ),
    },
    {
      key: "leads",
      header: t("leadSources.colLeads"),
      align: "end",
      render: (found) => <LeadCount count={found.lead_count} />,
    },
  ];
  if (canCreate) {
    columns.push({
      key: "adopt",
      header: t("leadSources.colActions"),
      headerHidden: true,
      fold: "end",
      render: (found) => (
        <Button
          onClick={() =>
            onAdopt(found, sourceKeyLabel(found.key, administered, t))
          }
        >
          {t("leadSources.adopt")}
        </Button>
      ),
    });
  }
  return (
    <>
      <PanelGroupHead title={t("leadSources.discovered")} level="h3" />
      <PanelBody>
        <p className="t-caption">{t("leadSources.discoveredSub")}</p>
      </PanelBody>
      <DataTable
        bleed
        fold
        label={t("leadSources.discovered")}
        columns={columns}
        rows={[...discovered]}
        rowKey={(found) => found.key}
        rowTestId={(found) => `lead-source-discovered-${found.key}`}
      />
    </>
  );
}

export function LeadSourcesCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const canRemove = useCanWrite("custom_field", "delete");
  const query = useLeadSources();
  const { create, adopt, update, rename, remove } = useSourceMutations();
  const [adding, setAdding] = useState(false);
  const [renaming, setRenaming] = useState<LeadSource | null>(null);
  const [removing, setRemoving] = useState<LeadSource | null>(null);
  const failure = [adopt, update].find((m) => m.isError);
  const administered = rowsOf(query.data?.data);
  const discovered = rowsOf(query.data?.discovered);
  const columns = sourceColumns({
    t,
    plural,
    locale,
    grants: { canEdit, canRemove },
    onUpdate: (patch) => update.mutate(patch),
    onRename: (source) => {
      rename.reset();
      setRenaming(source);
    },
    onRemove: setRemoving,
  });
  return (
    <Panel
      title={t("leadSources.title")}
      titleAction={
        canCreate && (
          <Button onClick={() => setAdding(true)}>
            {t("leadSources.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("leadSources.sub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && administered.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("leadSources.title")}
          columns={columns}
          rows={[...administered]}
          rowKey={(source) => source.id}
          rowTestId={(source) => `lead-source-${source.key}`}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("leadSources.title")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      {discovered.length > 0 && (
        <DiscoveredSources
          discovered={discovered}
          administered={administered}
          canCreate={canCreate}
          onAdopt={(found, label) =>
            adopt.mutate({ key: found.key, label, intent: "neutral" })
          }
        />
      )}
      <VocabNotices readOnly={!canEdit} error={failure?.error} />
      {adding && (
        <AddSourceDialog
          create={create}
          onClose={() => {
            create.reset();
            setAdding(false);
          }}
        />
      )}
      <NameDialog
        open={renaming !== null}
        onClose={() => setRenaming(null)}
        title={t("leadSources.renameTitle")}
        label={t("leadSources.labelField")}
        initial={renaming?.label ?? ""}
        confirmLabel={t("leadSources.renameSave")}
        pending={rename.isPending}
        problem={rename.isError ? problemMessageOf(rename.error, t) : null}
        onSave={(label) => {
          if (renaming) {
            rename.mutate(
              { id: renaming.id, label },
              { onSuccess: () => setRenaming(null) },
            );
          }
        }}
      />
      <ConfirmModal
        open={removing !== null}
        onClose={() => {
          remove.reset();
          setRemoving(null);
        }}
        title={t("leadSources.removeTitle")}
        confirmLabel={t("leadSources.remove")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() => {
          if (removing) {
            remove.mutate(removing.id, { onSuccess: () => setRemoving(null) });
          }
        }}
      >
        <p>{t("leadSources.removeBody", { label: removing?.label ?? "" })}</p>
      </ConfirmModal>
    </Panel>
  );
}
