// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { rowsOf } from "../../api/rows";
import { Button, EmptyState } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { KeyCode } from "../../design-system/keyedname";
import { NameDialog, nameRefusal } from "../../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryStates } from "../../design-system/querystates";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import {
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { Locale } from "../../i18n/locale";
import {
  type AcquisitionSource,
  useAcquisitionSources,
  useCreateAcquisitionSource,
  useUpdateAcquisitionSource,
} from "../../lib/acquisitionsources.queries";
import { problemMessageOf } from "../../lib/problem";
import { useVocabGrants, vocabMenuColumn } from "../leads/leadvocab.rows";

function sourceColumns({
  t,
  plural,
  locale,
  canEdit,
  refusedFor,
  onActive,
  onRename,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  canEdit: boolean;
  refusedFor: (source: AcquisitionSource) => string | undefined;
  onActive: (source: AcquisitionSource, active: boolean) => void;
  onRename: (source: AcquisitionSource) => void;
}>): DataTableColumn<AcquisitionSource>[] {
  const name: DataTableColumn<AcquisitionSource> = {
    key: "name",
    header: t("acqSources.colSource"),
    grow: true,
    render: (source) => source.label,
  };
  const deals: DataTableColumn<AcquisitionSource> = {
    key: "deals",
    header: t("acqSources.colDeals"),
    align: "end",
    render: (source) =>
      typeof source.deal_count === "number" ? (
        formatNumber(source.deal_count, locale)
      ) : (
        <DealsWithheld />
      ),
    folded: (source) =>
      typeof source.deal_count === "number"
        ? plural("acqSources.deals", source.deal_count, {
            count: formatNumber(source.deal_count, locale),
          })
        : undefined,
  };
  const active: DataTableColumn<AcquisitionSource> = {
    key: "active",
    header: t("leadSources.colActive"),
    fold: "end",
    render: (source) => (
      <Switch
        describedBy={refusedFor(source)}
        label={t("acqSources.activeFor", { label: source.label })}
        labelHidden
        checked={source.active}
        disabled={!canEdit}
        onChange={(next) => onActive(source, next)}
      />
    ),
  };
  return [
    name,
    deals,
    active,
    ...vocabMenuColumn(
      t,
      { canEdit, canRemove: false },
      (source: AcquisitionSource) => ({
        label: source.label,
        onRename: () => onRename(source),
      }),
    ),
  ];
}

// A seat that may not read deals is sent no count, and a zero would claim none.
export function DealsWithheld() {
  const t = useT();
  return (
    <span title={t("acqSources.dealsWithheld")}>
      <span aria-hidden="true">—</span>
      <span className="sr-only">{t("acqSources.dealsWithheld")}</span>
    </span>
  );
}

type Naming =
  | { mode: "create" }
  | { mode: "rename"; source: AcquisitionSource };

// Business channels a deal is attributed to, apart from lead sources, which
// record how a record reached Margince. No delete: a key a deal carried must
// stay resolvable, so the switch retires it from pickers instead.
export function AcquisitionSourcesCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { canCreate, canEdit, refusal } = useVocabGrants("acquisition");
  const query = useAcquisitionSources();
  const create = useCreateAcquisitionSource();
  const update = useUpdateAcquisitionSource();
  const rename = useUpdateAcquisitionSource();
  const refusalId = useId();
  // Kept after close, so the dialog leaving the page still reads as what it was.
  const [naming, setNaming] = useState<Naming>({ mode: "create" });
  const [namingOpen, setNamingOpen] = useState(false);
  const sources = rowsOf(query.data);
  const write = naming.mode === "rename" ? rename : create;
  // A rename cannot collide: only the key is unique, and it never changes.
  const refused =
    naming.mode === "rename"
      ? {
          problem: rename.isError ? problemMessageOf(rename.error, t) : null,
          nameProblem: null,
        }
      : nameRefusal(create.error, t, "acqSources.duplicate");
  const open = (next: Naming) => {
    create.reset();
    rename.reset();
    setNaming(next);
    setNamingOpen(true);
  };
  const columns = sourceColumns({
    t,
    plural,
    locale,
    canEdit,
    refusedFor: (source) =>
      update.isError && update.variables?.id === source.id
        ? refusalId
        : undefined,
    onActive: (source, active) =>
      update.mutate({ id: source.id, body: { active } }),
    onRename: (source) => open({ mode: "rename", source }),
  });
  const done = { onSuccess: () => setNamingOpen(false) };
  return (
    <Panel
      title={t("acqSources.title")}
      titleAction={
        canCreate && (
          <Button onClick={() => open({ mode: "create" })}>
            {t("acqSources.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("acqSources.sub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && sources.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("acqSources.title")}
          columns={columns}
          rows={[...sources]}
          rowKey={(source) => source.id}
          rowTestId={(source) => `acq-source-${source.key}`}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("acqSources.loading")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <PanelNotices
        readOnly={refusal && t(refusal)}
        refused={
          update.isError
            ? {
                title: t("leadSources.notSaved"),
                error: update.error,
                id: refusalId,
              }
            : undefined
        }
      />
      <NameDialog
        open={namingOpen}
        onClose={() => setNamingOpen(false)}
        title={t(
          naming.mode === "rename"
            ? "acqSources.renameTitle"
            : "acqSources.addTitle",
        )}
        label={t("acqSources.addLabel")}
        initial={naming.mode === "rename" ? naming.source.label : ""}
        hint={naming.mode === "rename" && <KeyCode code={naming.source.key} />}
        confirmLabel={t(
          naming.mode === "rename"
            ? "leadSources.renameSave"
            : "acqSources.addConfirm",
        )}
        pending={write.isPending}
        problem={refused.problem}
        nameProblem={refused.nameProblem}
        onSave={(label) => {
          if (naming.mode === "rename") {
            rename.mutate({ id: naming.source.id, body: { label } }, done);
          } else {
            create.mutate({ label }, done);
          }
        }}
      />
    </Panel>
  );
}
