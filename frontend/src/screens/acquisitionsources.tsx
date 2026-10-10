// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { rowsOf } from "../api/rows";
import { useCanWrite } from "../app/capability";
import { Button, EmptyState, OverflowMenu } from "../design-system/atoms";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { KeyedName } from "../design-system/keyedname";
import { NameDialog, nameRefusal } from "../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { PanelNotices } from "../design-system/panelnotices";
import { Switch } from "../design-system/switch";
import { formatNumber } from "../format/format";
import {
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { Locale } from "../i18n/locale";
import {
  type AcquisitionSource,
  useAcquisitionSources,
  useCreateAcquisitionSource,
  useUpdateAcquisitionSource,
} from "./acquisitionsources.queries";
import { QueryStates } from "./common";

function sourceColumns({
  t,
  plural,
  locale,
  canEdit,
  onActive,
  onRename,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  canEdit: boolean;
  onActive: (source: AcquisitionSource, active: boolean) => void;
  onRename: (source: AcquisitionSource) => void;
}>): DataTableColumn<AcquisitionSource>[] {
  const name: DataTableColumn<AcquisitionSource> = {
    key: "name",
    header: t("acqSources.colSource"),
    grow: true,
    render: (source) => <KeyedName name={source.label} code={source.key} />,
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
    render: (source) => (
      <Switch
        label={t("acqSources.activeFor", { label: source.label })}
        labelHidden
        checked={source.active}
        disabled={!canEdit}
        onChange={(next) => onActive(source, next)}
      />
    ),
  };
  const verbs: DataTableColumn<AcquisitionSource> = {
    key: "verbs",
    header: t("leadSources.colActions"),
    headerHidden: true,
    fold: "end",
    render: (source) => (
      <span className="cell-actions">
        <OverflowMenu label={t("table.rowActions", { name: source.label })}>
          <Button onClick={() => onRename(source)}>
            {t("leadSources.rename")}
          </Button>
        </OverflowMenu>
      </span>
    ),
  };
  return canEdit ? [name, deals, active, verbs] : [name, deals, active];
}

// A seat that may not read deals is sent no count, and a zero would claim none.
function DealsWithheld() {
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
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const query = useAcquisitionSources();
  const create = useCreateAcquisitionSource();
  const update = useUpdateAcquisitionSource();
  const rename = useUpdateAcquisitionSource();
  // Kept after close, so the dialog leaving the page still reads as what it was.
  const [naming, setNaming] = useState<Naming>({ mode: "create" });
  const [namingOpen, setNamingOpen] = useState(false);
  const sources = rowsOf(query.data);
  const write = naming.mode === "rename" ? rename : create;
  const refused = nameRefusal(write.error, t, "acqSources.duplicate");
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
        readOnly={!canEdit && t("leadSources.readOnlyTitle")}
        refused={
          update.isError
            ? { title: t("leadSources.notSaved"), error: update.error }
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
