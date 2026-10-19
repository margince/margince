// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useId, useState } from "react";
import { rowsOf } from "../../api/rows";
import {
  Button,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { describedByIds } from "../../design-system/describedby";
import { Heading } from "../../design-system/heading";
import { KeyCode, KeyedName } from "../../design-system/keyedname";
import {
  NameDialog,
  nameRefusal,
  useRefusedName,
} from "../../design-system/namedialog";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { useSinglePress } from "../../design-system/presslatch";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import {
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Locale } from "../../i18n/locale";
import { isOption } from "../../lib/options";
import { problemMessageOf } from "../../lib/problem";
import {
  type DiscoveredLeadSource,
  type LeadSource,
  type LeadSourceIntent,
  type SourcePatch,
  sourceKeyLabel,
  useLeadSources,
  useSourceMutations,
} from "./leadsources";
import {
  leadsColumn,
  useRemovalFocus,
  useVocabGrants,
  vocabMenuColumn,
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
  const name = label.trim();
  const refused = nameRefusal(create.error, t, "leadSources.duplicate");
  const { refusedName, markSent } = useRefusedName(name, refused.nameProblem);
  const ready = name !== "" && !refusedName;
  const singlePress = useSinglePress(create.isPending);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" className="t-h2 modal-title" id={titleId}>
        {t("leadSources.newLabel")}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={singlePress((e: FormEvent) => {
          e.preventDefault();
          if (!ready || create.isPending) return;
          markSent();
          create.mutate({ label: name, intent }, { onSuccess: onClose });
        })}
      >
        {refused.problem && (
          <Callout
            kind="outcome"
            tone="danger"
            title={t("leadSources.notAdded")}
          >
            {refused.problem}
          </Callout>
        )}
        <Field label={t("leadSources.labelField")} error={refusedName}>
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
  refusedFor,
  onUpdate,
  onRename,
  onRemove,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  grants: Readonly<{ canEdit: boolean; canRemove: boolean }>;
  refusedFor: (
    source: LeadSource,
    field: "intent" | "active",
  ) => string | undefined;
  onUpdate: (patch: SourcePatch) => void;
  onRename: (source: LeadSource) => void;
  onRemove: (source: LeadSource) => void;
}>): DataTableColumn<LeadSource>[] {
  const { canEdit } = grants;
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
      render: (source) => source.label,
    },
    leadsColumn({ t, plural, locale }, (source) => source.lead_count ?? 0),
    {
      key: "intent",
      header: t("leadSources.intent"),
      render: (source) => (
        <span className="lead-vocab-intent">
          <Select
            aria-label={t("leadSources.intentFor", { label: source.label })}
            aria-describedby={refusedFor(source, "intent")}
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
      fold: "end",
      render: (source) => (
        <Switch
          describedBy={refusedFor(source, "active")}
          label={t("leadSources.activeFor", { label: source.label })}
          labelHidden
          checked={source.active}
          disabled={!canEdit}
          onChange={(next) => onUpdate({ id: source.id, active: next })}
        />
      ),
    },
    ...vocabMenuColumn(t, grants, (source: LeadSource) => ({
      label: source.label,
      refusal: refusal(source),
      onRename: () => onRename(source),
      onRemove: () => onRemove(source),
    })),
  ];
}

// Values a lead carries that the list does not name yet, one verb each.
function DiscoveredSources({
  discovered,
  administered,
  canCreate,
  refusedFor,
  onAdopt,
}: Readonly<{
  discovered: readonly DiscoveredLeadSource[];
  administered: readonly LeadSource[];
  canCreate: boolean;
  refusedFor: (found: DiscoveredLeadSource) => string | undefined;
  onAdopt: (found: DiscoveredLeadSource, label: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const tableId = useId();
  // By position: a discovered key is whatever a lead carried, spaces included.
  const nameId = (found: DiscoveredLeadSource) =>
    `${tableId}-${discovered.indexOf(found)}`;
  const columns: DataTableColumn<DiscoveredLeadSource>[] = [
    {
      key: "name",
      header: t("leadSources.colSource"),
      grow: true,
      render: (found) => (
        <span id={nameId(found)}>
          <KeyedName
            name={sourceKeyLabel(found.key, administered, t)}
            code={found.key}
          />
        </span>
      ),
    },
    leadsColumn({ t, plural, locale }, (found) => found.lead_count),
  ];
  if (canCreate) {
    columns.push({
      key: "adopt",
      header: t("leadSources.colActions"),
      headerHidden: true,
      fold: "end",
      render: (found) => (
        <Button
          aria-describedby={describedByIds(nameId(found), refusedFor(found))}
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
  const { canCreate, canEdit, canRemove, refusal } = useVocabGrants("leads");
  const query = useLeadSources();
  const { create, adopt, update, rename, remove } = useSourceMutations();
  const [adding, setAdding] = useState(false);
  const [renaming, setRenaming] = useState<LeadSource | null>(null);
  const [removing, setRemoving] = useState<LeadSource | null>(null);
  const removal = useRemovalFocus();
  const failure = [adopt, update]
    .filter((m) => m.isError)
    .sort((a, b) => b.submittedAt - a.submittedAt)[0];
  const refusalId = useId();
  const refusedPatch = failure === update ? update.variables : undefined;
  const refusedAdopt = failure === adopt ? adopt.variables?.key : undefined;
  const administered = rowsOf(query.data?.data);
  const discovered = rowsOf(query.data?.discovered);
  const columns = sourceColumns({
    t,
    plural,
    locale,
    grants: { canEdit, canRemove },
    refusedFor: (source, field) =>
      refusedPatch?.id === source.id && refusedPatch[field] !== undefined
        ? refusalId
        : undefined,
    onUpdate: (patch) => update.mutate(patch),
    onRename: (source) => {
      rename.reset();
      setRenaming(source);
    },
    onRemove: (source) => {
      removal.opened();
      setRemoving(source);
    },
  });
  return (
    <Panel
      title={
        <span id={removal.titleId} tabIndex={-1}>
          {t("leadSources.title")}
        </span>
      }
      titleAction={
        canCreate && (
          <Button ref={removal.addVerb} onClick={() => setAdding(true)}>
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
          refusedFor={(found) =>
            refusedAdopt === found.key ? refusalId : undefined
          }
          onAdopt={(found, label) =>
            adopt.mutate({ key: found.key, label, intent: "neutral" })
          }
        />
      )}
      <PanelNotices
        readOnly={refusal && t(refusal)}
        refused={
          failure && {
            title: t("leadSources.notSaved"),
            error: failure.error,
            id: refusalId,
          }
        }
      />
      {adding && (
        <AddSourceDialog
          create={create}
          onClose={() => {
            create.reset();
            setAdding(false);
          }}
        />
      )}
      {/* A rename cannot collide: only the key is unique, and it never changes. */}
      <NameDialog
        open={renaming !== null}
        onClose={() => setRenaming(null)}
        title={t("leadSources.renameTitle")}
        label={t("leadSources.labelField")}
        initial={renaming?.label ?? ""}
        hint={renaming && <KeyCode code={renaming.key} />}
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
        returnFocusTo={removal.returnFocusTo}
        title={t("leadSources.removeTitle")}
        confirmLabel={t("leadSources.remove")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() => {
          if (removing) {
            remove.mutate(removing.id, {
              onSuccess: () => {
                removal.landed();
                setRemoving(null);
              },
            });
          }
        }}
      >
        <p>{t("leadSources.removeBody", { label: removing?.label ?? "" })}</p>
      </ConfirmModal>
    </Panel>
  );
}
