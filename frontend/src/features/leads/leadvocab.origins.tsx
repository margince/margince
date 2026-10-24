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
import { Heading } from "../../design-system/heading";
import { KeyCode } from "../../design-system/keyedname";
import {
  NameDialog,
  nameRefusal,
  useRefusedName,
} from "../../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { useSinglePress } from "../../design-system/presslatch";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { Switch } from "../../design-system/switch";
import { useToast } from "../../design-system/toast";
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
import {
  type LeadOrigin,
  type LeadOriginIntent,
  type OriginPatch,
  useLeadOrigins,
  useOriginMutations,
} from "../../lib/leadorigins";
import { isOption } from "../../lib/options";
import { problemMessageOf } from "../../lib/problem";
import {
  leadsColumn,
  useRemovalFocus,
  useVocabGrants,
  vocabMenuColumn,
} from "./leadvocab.rows";

const INTENTS = ["high", "neutral", "low"] as const;

const intentLabel: Record<LeadOriginIntent, MessageKey> = {
  high: "leadSources.intent.high",
  neutral: "leadSources.intent.neutral",
  low: "leadSources.intent.low",
};

function intentOptions(t: Translator) {
  return INTENTS.map((value) => ({ value, label: t(intentLabel[value]) }));
}

// Mounted only while open, so a half-typed origin never waits under an intent
// nobody re-chose.
function AddOriginDialog({
  create,
  onClose,
}: Readonly<{
  create: ReturnType<typeof useOriginMutations>["create"];
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  const [label, setLabel] = useState("");
  const [intent, setIntent] = useState<LeadOriginIntent>("neutral");
  const name = label.trim();
  const refused = nameRefusal(create.error, t, "leadOrigins.duplicate");
  const { refusedName, markSent } = useRefusedName(name, refused.nameProblem);
  const ready = name !== "" && !refusedName;
  const singlePress = useSinglePress(create.isPending);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" className="t-h2 modal-title" id={titleId}>
        {t("leadOrigins.newLabel")}
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
            title={t("leadOrigins.notAdded")}
          >
            {refused.problem}
          </Callout>
        )}
        <Field label={t("leadSources.labelField")} error={refusedName}>
          {(control) => (
            <TextInput
              {...control}
              data-testid="lead-origin-new-label"
              placeholder={t("leadOrigins.newPlaceholder")}
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
          {t("leadOrigins.add")}
        </Button>
      </div>
    </Modal>
  );
}

function originColumns({
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
    origin: LeadOrigin,
    field: "intent" | "active",
  ) => string | undefined;
  onUpdate: (patch: OriginPatch) => void;
  onRename: (origin: LeadOrigin) => void;
  onRemove: (origin: LeadOrigin) => void;
}>): DataTableColumn<LeadOrigin>[] {
  const { canEdit } = grants;
  const refusal = (origin: LeadOrigin) => {
    const count = origin.lead_count ?? 0;
    if (origin.system === true) return t("leadOrigins.builtInKept");
    if (count > 0) {
      return plural("leadOrigins.inUse", count, {
        count: formatNumber(count, locale),
      });
    }
    return undefined;
  };
  return [
    {
      key: "name",
      header: t("leadOrigins.colOrigin"),
      grow: true,
      render: (origin) => origin.label,
    },
    leadsColumn({ t, plural, locale }, (origin) => origin.lead_count ?? 0),
    {
      key: "intent",
      header: t("leadSources.intent"),
      render: (origin) => (
        <span className="lead-vocab-intent">
          <Select
            aria-label={t("leadSources.intentFor", { label: origin.label })}
            aria-describedby={refusedFor(origin, "intent")}
            value={origin.intent}
            disabled={!canEdit}
            onChange={(value) => {
              if (isOption(value, INTENTS) && value !== origin.intent) {
                onUpdate({ id: origin.id, intent: value });
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
      render: (origin) => (
        <Switch
          describedBy={refusedFor(origin, "active")}
          label={t("leadSources.activeFor", { label: origin.label })}
          labelHidden
          checked={origin.active}
          disabled={!canEdit}
          onChange={(next) => onUpdate({ id: origin.id, active: next })}
        />
      ),
    },
    ...vocabMenuColumn(t, grants, (origin: LeadOrigin) => ({
      label: origin.label,
      refusal: refusal(origin),
      onRename: () => onRename(origin),
      onRemove: () => onRemove(origin),
    })),
  ];
}

export function LeadOriginsCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { canCreate, canEdit, canRemove, refusal } = useVocabGrants("leads");
  const query = useLeadOrigins();
  const { create, update, rename, remove } = useOriginMutations();
  const [adding, setAdding] = useState(false);
  const [renaming, setRenaming] = useState<LeadOrigin | null>(null);
  const [removing, setRemoving] = useState<LeadOrigin | null>(null);
  const removal = useRemovalFocus();
  const failure = update.isError ? update : undefined;
  const toast = useToast();
  const refusalId = useId();
  const refusedPatch = failure ? update.variables : undefined;
  const administered = rowsOf(query.data);
  const columns = originColumns({
    t,
    plural,
    locale,
    grants: { canEdit, canRemove },
    refusedFor: (origin, field) =>
      refusedPatch?.id === origin.id && refusedPatch[field] !== undefined
        ? refusalId
        : undefined,
    // A row's controls write the moment they change, so each write confirms itself.
    onUpdate: (patch) =>
      update.mutate(patch, {
        onSuccess: () => toast.show(t("settings.saved")),
      }),
    onRename: (origin) => {
      rename.reset();
      setRenaming(origin);
    },
    onRemove: (origin) => {
      removal.opened();
      setRemoving(origin);
    },
  });
  return (
    <Panel
      title={
        <span id={removal.titleId} tabIndex={-1}>
          {t("leadOrigins.title")}
        </span>
      }
      titleAction={
        canCreate && (
          <Button ref={removal.addVerb} onClick={() => setAdding(true)}>
            {t("leadOrigins.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("leadOrigins.sub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && administered.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("leadOrigins.title")}
          columns={columns}
          rows={[...administered]}
          rowKey={(origin) => origin.id}
          rowTestId={(origin) => `lead-origin-${origin.key}`}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("leadOrigins.title")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
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
        <AddOriginDialog
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
        title={t("leadOrigins.renameTitle")}
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
        title={t("leadOrigins.removeTitle")}
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
        <p>{t("leadOrigins.removeBody", { label: removing?.label ?? "" })}</p>
      </ConfirmModal>
    </Panel>
  );
}
